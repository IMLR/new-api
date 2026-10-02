package model

import (
	"errors"
	"sort"

	"github.com/QuantumNous/new-api/common"
)

// modelRoutingMatchers shares the channel cache lock. Rules belong to the
// requested model's exact metadata record, never to another overlapping rule.
var modelRoutingMatchers map[string]*ModelMatcher

func loadModelRoutingMatchers() (map[string]*ModelMatcher, error) {
	var models []Model
	if err := DB.Where("status = ? AND match_rule IS NOT NULL", 1).Find(&models).Error; err != nil {
		return nil, err
	}
	matchers := make(map[string]*ModelMatcher, len(models))
	for _, metadata := range models {
		matcher, err := metadata.CompileMatcher()
		if err != nil {
			return nil, err
		}
		matchers[metadata.ModelName] = matcher
	}
	return matchers, nil
}

func getModelRoutingMatcherDB(modelName string) (*ModelMatcher, error) {
	var metadata Model
	err := DB.Where("model_name = ? AND status = ?", modelName, 1).Limit(1).Find(&metadata).Error
	if err != nil || metadata.MatchRule == nil {
		return nil, err
	}
	return metadata.CompileMatcher()
}

// matchingChannelModels picks one available native model per account. Exact
// names come first, then lexical order; extra variants never multiply weight.
// Caller holds channelSyncLock. Both client and native cooldowns are respected.
func matchingChannelModels(group, modelName, requestPath string, usedChannelIDs []int) map[int]string {
	matcher := modelRoutingMatchers[modelName]
	models := make([]string, 0)
	for name := range group2model2channels[group] {
		if matcher.Matches(name) {
			models = append(models, name)
		}
	}
	sort.Strings(models)
	for i, name := range models {
		if name == modelName {
			models = append([]string{name}, append(models[:i], models[i+1:]...)...)
			break
		}
	}
	used := toChannelIdSet(usedChannelIDs)
	routes := make(map[int]string)
	for _, name := range models {
		for _, id := range filterChannelsByRequestPathAndModel(group2model2channels[group][name], requestPath, name) {
			if _, exists := routes[id]; exists {
				continue
			}
			if channelModelAvailable(id, modelName, used) && !IsChannelModelCoolingDown(id, name) {
				routes[id] = name
			}
		}
	}
	return routes
}

func getModelRoutingAbilities(group, modelName, requestPath string) ([]Ability, error) {
	matcher, err := getModelRoutingMatcherDB(modelName)
	if err != nil {
		return nil, err
	}
	query := DB.Where(commonGroupCol+" = ? AND enabled = ?", group, true)
	if matcher == nil {
		query = query.Where("model = ?", modelName)
	}
	var abilities []Ability
	if err := query.Order("weight DESC").Find(&abilities).Error; err != nil {
		return nil, err
	}
	if matcher != nil {
		filtered := make([]Ability, 0, len(abilities))
		for _, ability := range abilities {
			if matcher.Matches(ability.Model) {
				filtered = append(filtered, ability)
			}
		}
		abilities = filtered
		sort.SliceStable(abilities, func(i, j int) bool {
			if (abilities[i].Model == modelName) != (abilities[j].Model == modelName) {
				return abilities[i].Model == modelName
			}
			return abilities[i].Model < abilities[j].Model
		})
	}
	return filterAbilitiesByRequestPathAndModel(abilities, requestPath, modelName), nil
}

// ResolveChannelModel supplies the native name after selection without
// modifying shared channel objects or their explicit model mappings.
func ResolveChannelModel(group, modelName string, channelID int, requestPath string) (string, error) {
	if group == "" {
		return modelName, nil
	}
	if common.MemoryCacheEnabled {
		channelSyncLock.RLock()
		defer channelSyncLock.RUnlock()
		if modelRoutingMatchers[modelName] == nil {
			return modelName, nil
		}
		if name, ok := matchingChannelModels(group, modelName, requestPath, nil)[channelID]; ok {
			return name, nil
		}
	} else {
		matcher, err := getModelRoutingMatcherDB(modelName)
		if err != nil {
			return "", err
		}
		if matcher == nil {
			return modelName, nil
		}
		abilities, err := getModelRoutingAbilities(group, modelName, requestPath)
		if err != nil {
			return "", err
		}
		for _, ability := range filterUnavailableAbilities(abilities, modelName, nil) {
			if ability.ChannelId == channelID {
				return ability.Model, nil
			}
		}
	}
	return "", errors.New("selected channel has no available matching model")
}
