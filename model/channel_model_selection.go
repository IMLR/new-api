package model

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ChannelModelExclusion survives rebuilding abilities, channel status changes,
// and model metadata deletion. It disables this channel/model in every group.
type ChannelModelExclusion struct {
	ChannelId int    `gorm:"primaryKey;autoIncrement:false"`
	Model     string `gorm:"type:varchar(255);primaryKey;autoIncrement:false"`
}

type ChannelModelSelection struct {
	ChannelId int    `json:"channel_id"`
	Model     string `json:"model"`
	Selected  bool   `json:"selected"`
}

type ModelPreviewChannel struct {
	Id       int      `json:"id"`
	Name     string   `json:"name"`
	Type     int      `json:"type"`
	Groups   []string `json:"groups"`
	Enabled  bool     `json:"enabled"`
	Selected bool     `json:"selected"`
	Priority int64    `json:"priority"`
}

type ModelMatchPreview struct {
	Model    string                `json:"model"`
	Channels []ModelPreviewChannel `json:"channels"`
}

func GetModelMatchPreview(mi *Model) ([]ModelMatchPreview, error) {
	matcher, err := mi.CompileMatcher()
	if err != nil {
		return nil, err
	}
	var channels []Channel
	if err := DB.Select("id", "name", "type", "models", "group", "status", "priority").Order("id ASC").Find(&channels).Error; err != nil {
		return nil, err
	}
	excluded, err := getAllChannelModelExclusions(DB)
	if err != nil {
		return nil, err
	}
	byModel := make(map[string][]ModelPreviewChannel)
	for _, channel := range channels {
		seen := make(map[string]bool)
		for _, name := range strings.Split(channel.Models, ",") {
			name = strings.TrimSpace(name)
			if name == "" || seen[name] || !matcher.Matches(name) {
				continue
			}
			seen[name] = true
			byModel[name] = append(byModel[name], ModelPreviewChannel{
				Id: channel.Id, Name: channel.Name, Type: channel.Type,
				Groups:   strings.Split(channel.Group, ","),
				Enabled:  channel.Status == common.ChannelStatusEnabled,
				Selected: !excluded[channel.Id][name], Priority: channel.GetPriority(),
			})
		}
	}
	preview := make([]ModelMatchPreview, 0, len(byModel))
	for name, channels := range byModel {
		sort.SliceStable(channels, func(i, j int) bool { return channels[i].Priority > channels[j].Priority })
		preview = append(preview, ModelMatchPreview{Model: name, Channels: channels})
	}
	sort.Slice(preview, func(i, j int) bool { return preview[i].Model < preview[j].Model })
	return preview, nil
}

func (mi *Model) SaveWithChannelSelections(create bool) error {
	if _, err := mi.CompileMatcher(); err != nil {
		return err
	}
	if mi.MatchRule != nil {
		mi.NameRule = NameRuleContains
	}
	if len(mi.ChannelSelections) > 4096 {
		return errors.New("too many channel model selections")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if create {
			if err := mi.insert(tx); err != nil {
				return err
			}
		} else {
			var existing Model
			if err := tx.First(&existing, mi.Id).Error; err != nil {
				return err
			}
			if err := mi.update(tx); err != nil {
				return err
			}
		}
		return SetChannelModelSelections(tx, mi.ChannelSelections)
	})
}

func SetChannelModelSelections(tx *gorm.DB, selections []ChannelModelSelection) error {
	if len(selections) == 0 {
		return nil
	}
	ids := make([]int, 0)
	seen := make(map[int]bool)
	for _, selection := range selections {
		if selection.ChannelId <= 0 || selection.Model == "" {
			return errors.New("invalid channel model selection")
		}
		if !seen[selection.ChannelId] {
			ids = append(ids, selection.ChannelId)
			seen[selection.ChannelId] = true
		}
	}
	sort.Ints(ids)
	var channels []Channel
	if err := lockForUpdate(tx).Where("id IN ?", ids).Order("id ASC").Find(&channels).Error; err != nil {
		return err
	}
	byID := make(map[int]Channel, len(channels))
	for _, channel := range channels {
		byID[channel.Id] = channel
	}
	for _, selection := range selections {
		channel, exists := byID[selection.ChannelId]
		if !exists || !common.StringsContains(strings.Split(channel.Models, ","), selection.Model) {
			return fmt.Errorf("channel %d no longer contains model %s; refresh the preview", selection.ChannelId, selection.Model)
		}
		exclusion := ChannelModelExclusion{ChannelId: selection.ChannelId, Model: selection.Model}
		if selection.Selected {
			if err := tx.Where("channel_id = ? AND model = ?", selection.ChannelId, selection.Model).Delete(&ChannelModelExclusion{}).Error; err != nil {
				return err
			}
		} else if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&exclusion).Error; err != nil {
			return err
		}
		enabled := selection.Selected && channel.Status == common.ChannelStatusEnabled
		if err := tx.Model(&Ability{}).Where("channel_id = ? AND model = ?", channel.Id, selection.Model).Update("enabled", enabled).Error; err != nil {
			return err
		}
	}
	return nil
}

func getChannelModelExclusions(tx *gorm.DB, channelID int) (map[string]bool, error) {
	var exclusions []ChannelModelExclusion
	if err := tx.Where("channel_id = ?", channelID).Find(&exclusions).Error; err != nil {
		return nil, err
	}
	result := make(map[string]bool, len(exclusions))
	for _, exclusion := range exclusions {
		result[exclusion.Model] = true
	}
	return result, nil
}

func getAllChannelModelExclusions(tx *gorm.DB) (map[int]map[string]bool, error) {
	var exclusions []ChannelModelExclusion
	if err := tx.Find(&exclusions).Error; err != nil {
		return nil, err
	}
	excluded := make(map[int]map[string]bool)
	for _, exclusion := range exclusions {
		if excluded[exclusion.ChannelId] == nil {
			excluded[exclusion.ChannelId] = make(map[string]bool)
		}
		excluded[exclusion.ChannelId][exclusion.Model] = true
	}
	return excluded, nil
}

func updateAbilityStatus(query *gorm.DB, status bool) error {
	return query.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Ability{}).Update("enabled", status).Error; err != nil {
			return err
		}
		if !status {
			return nil
		}
		// Keep user exclusions when a whole channel or tag is re-enabled.
		return tx.Model(&Ability{}).
			Where("EXISTS (?)", DB.Model(&ChannelModelExclusion{}).Select("1").
				Where("channel_model_exclusions.channel_id = abilities.channel_id AND channel_model_exclusions.model = abilities.model")).
			Update("enabled", false).Error
	})
}
