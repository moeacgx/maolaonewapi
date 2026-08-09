package model

import (
	"sort"
	"strings"
)

type ChannelOption struct {
	Id     int     `json:"id"`
	Name   string  `json:"name"`
	Status int     `json:"status"`
	Type   int     `json:"type"`
	Tag    *string `json:"tag"`
}

type ChannelTagOption struct {
	Tag          string `json:"tag"`
	ChannelCount int    `json:"channel_count"`
}

// GetAllChannelOptions 只读取渠道选择器需要的非敏感字段和管理标签。
func GetAllChannelOptions() ([]ChannelOption, error) {
	options := make([]ChannelOption, 0)
	err := DB.Model(&Channel{}).
		Select([]string{"id", "name", "status", "type", "tag"}).
		Where("id > ?", 0).
		Order("id ASC").
		Find(&options).Error
	return options, err
}

// GetAllChannelTagOptions 按渠道管理的 Tag 字段汇总真实渠道分组。
// 在 Go 中去重可避免不同数据库默认排序规则造成大小写语义差异。
func GetAllChannelTagOptions() ([]ChannelTagOption, error) {
	var tags []*string
	if err := DB.Model(&Channel{}).
		Select("tag").
		Where("id > ? AND tag IS NOT NULL", 0).
		Pluck("tag", &tags).Error; err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(tags))
	for _, value := range tags {
		if value == nil {
			continue
		}
		tag := strings.TrimSpace(*value)
		if tag != "" {
			counts[tag]++
		}
	}
	tagNames := make([]string, 0, len(counts))
	for tag := range counts {
		tagNames = append(tagNames, tag)
	}
	sort.Strings(tagNames)
	options := make([]ChannelTagOption, 0, len(tagNames))
	for _, tag := range tagNames {
		options = append(options, ChannelTagOption{Tag: tag, ChannelCount: counts[tag]})
	}
	return options, nil
}
