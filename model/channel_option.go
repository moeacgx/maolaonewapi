package model

type ChannelOption struct {
	Id     int    `json:"id"`
	Name   string `json:"name"`
	Status int    `json:"status"`
	Type   int    `json:"type"`
}

// GetAllChannelOptions 只读取渠道选择器需要的非敏感字段。
func GetAllChannelOptions() ([]ChannelOption, error) {
	options := make([]ChannelOption, 0)
	err := DB.Model(&Channel{}).
		Select("id", "name", "status", "type").
		Where("id > ?", 0).
		Order("id ASC").
		Find(&options).Error
	return options, err
}
