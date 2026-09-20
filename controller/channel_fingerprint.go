package controller

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelfingerprint"
	"github.com/gin-gonic/gin"
)

type fingerprintJobKey struct {
	ChannelID int
	Model     string
}

type fingerprintJob struct {
	Model      string                       `json:"model"`
	Status     string                       `json:"status"`
	Reference  string                       `json:"reference"`
	Candidates []modelfingerprint.Candidate `json:"candidates"`
	Samples    []modelfingerprint.Sample    `json:"samples"`
	created    time.Time
}

// Results are local to this server process and expire after one hour.
var fingerprintJobs = struct {
	sync.Mutex
	jobs map[fingerprintJobKey]*fingerprintJob
}{jobs: make(map[fingerprintJobKey]*fingerprintJob)}

func GetChannelFingerprint(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channel, err := model.GetChannelById(id, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !channel.GetSetting().RelayDetection || !slices.Contains(channel.GetModels(), c.Query("model")) {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "Fingerprint result is unavailable"})
		return
	}
	fingerprintJobs.Lock()
	job := fingerprintJobs.jobs[fingerprintJobKey{id, c.Query("model")}]
	var result *fingerprintJob
	if job != nil && time.Since(job.created) < time.Hour {
		snapshot := *job
		result = &snapshot
	}
	fingerprintJobs.Unlock()
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// TestChannelFingerprint performs a bounded, explicit administrator-triggered probe.
func TestChannelFingerprint(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var input struct {
		Model  string `json:"model"`
		Family string `json:"family"`
	}
	if err = c.ShouldBindJSON(&input); err != nil {
		common.ApiError(c, err)
		return
	}
	if input.Family != "gpt" && input.Family != "claude" {
		common.ApiError(c, fmt.Errorf("unsupported reference family"))
		return
	}
	channel, err := model.GetChannelById(id, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !channel.GetSetting().RelayDetection {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "Relay detection is disabled"})
		return
	}
	if !slices.Contains(channel.GetModels(), input.Model) {
		common.ApiError(c, fmt.Errorf("model is not enabled on this channel"))
		return
	}
	if channel.Type != constant.ChannelTypeOpenAI && channel.Type != constant.ChannelTypeAnthropic {
		common.ApiError(c, fmt.Errorf("fingerprint probes support OpenAI and Anthropic channels"))
		return
	}
	userID, err := resolveChannelTestUserID(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	challenges, err := modelfingerprint.Challenges()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// Each worker owns a channel snapshot, including mutable multi-key state.
	channels := make([]*model.Channel, len(challenges))
	for i := range channels {
		channels[i], err = common.DeepCopy(channel)
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}
	source := "OpenRouter Claude reference (2026-09-20)"
	if input.Family == "gpt" {
		source = "ModelTrace GitHub reference (2026-09-20)"
	}
	key := fingerprintJobKey{id, input.Model}
	fingerprintJobs.Lock()
	if existing := fingerprintJobs.jobs[key]; existing != nil && existing.Status == "running" {
		snapshot := *existing
		fingerprintJobs.Unlock()
		c.JSON(http.StatusAccepted, gin.H{"success": true, "data": snapshot})
		return
	}
	running := 0
	for k, job := range fingerprintJobs.jobs {
		if job.Status == "running" {
			running++
		} else if time.Since(job.created) >= time.Hour {
			delete(fingerprintJobs.jobs, k)
		}
	}
	if running >= 8 {
		fingerprintJobs.Unlock()
		c.JSON(http.StatusTooManyRequests, gin.H{"success": false, "message": "Too many fingerprint tests are running"})
		return
	}
	if len(fingerprintJobs.jobs) >= 128 {
		var oldestKey fingerprintJobKey
		var oldest *fingerprintJob
		for k, job := range fingerprintJobs.jobs {
			if job.Status != "running" && (oldest == nil || job.created.Before(oldest.created)) {
				oldestKey, oldest = k, job
			}
		}
		if oldest != nil {
			delete(fingerprintJobs.jobs, oldestKey)
		}
	}
	job := &fingerprintJob{Model: input.Model, Status: "running", Reference: source,
		Candidates: []modelfingerprint.Candidate{}, Samples: []modelfingerprint.Sample{}, created: time.Now()}
	fingerprintJobs.jobs[key] = job
	snapshot := *job
	fingerprintJobs.Unlock()
	time.AfterFunc(time.Hour, func() {
		fingerprintJobs.Lock()
		defer fingerprintJobs.Unlock()
		if fingerprintJobs.jobs[key] == job {
			delete(fingerprintJobs.jobs, key)
		}
	})
	go func() {
		samples := collectFingerprintSamples(challenges, func(i int, challenge modelfingerprint.Challenge) modelfingerprint.Sample {
			ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
			defer cancel()
			result := testChannelWithPrompt(ctx, channels[i], userID, input.Model, "", false, challenge.Prompt)
			if result.localErr != nil || result.newAPIError != nil {
				return modelfingerprint.Sample{Error: "upstream_request_failed"}
			}
			text, parseErr := fingerprintResponseText(result.responseBody)
			if parseErr != nil {
				return modelfingerprint.Sample{Error: "invalid_response"}
			}
			sample, rankErr := modelfingerprint.Rank(text, input.Family, challenge.Count)
			if rankErr != nil {
				return modelfingerprint.Sample{Error: "ranking_failed"}
			}
			return sample
		})
		candidates := modelfingerprint.Aggregate(samples)
		fingerprintJobs.Lock()
		job.Samples, job.Candidates, job.Status = samples, candidates, "completed"
		fingerprintJobs.Unlock()
	}()
	c.JSON(http.StatusAccepted, gin.H{"success": true, "data": snapshot})
}

// Preserve challenge order even when individual upstream requests finish out of order.
func collectFingerprintSamples(challenges []modelfingerprint.Challenge, probe func(int, modelfingerprint.Challenge) modelfingerprint.Sample) []modelfingerprint.Sample {
	samples := make([]modelfingerprint.Sample, len(challenges))
	var workers sync.WaitGroup
	for i, challenge := range challenges {
		workers.Add(1)
		go func(i int, challenge modelfingerprint.Challenge) {
			defer workers.Done()
			defer func() {
				if recover() != nil {
					samples[i] = modelfingerprint.Sample{Error: "upstream_request_failed"}
				}
				samples[i].ID = challenge.ID
			}()
			samples[i] = probe(i, challenge)
		}(i, challenge)
	}
	workers.Wait()
	return samples
}

func fingerprintResponseText(body []byte) (string, error) {
	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Output []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := common.Unmarshal(body, &response); err != nil {
		return "", err
	}
	if len(response.Choices) > 0 {
		return response.Choices[0].Message.Content, nil
	}
	var text strings.Builder
	for _, part := range response.Content {
		if part.Type == "text" {
			text.WriteString(part.Text)
		}
	}
	for _, out := range response.Output {
		for _, part := range out.Content {
			if part.Type == "output_text" {
				text.WriteString(part.Text)
			}
		}
	}
	return text.String(), nil
}
