package setting

import (
	"fmt"
	"strconv"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"strings"
	"sync"
)

const DefaultMaxTokenAutoGroups = 5
const DefaultAutoGroupDescription = "自动选择最佳可用分组，失败时按配置顺序切换"

type AutoGroupConfig struct {
	UserSelectable bool   `json:"user_selectable"`
	Description    string `json:"description"`
}

var autoGroupConfig = AutoGroupConfig{Description: DefaultAutoGroupDescription}
var autoGroupConfigMutex sync.RWMutex

func NormalizeAutoGroupConfig(config AutoGroupConfig) AutoGroupConfig {
	config.Description = strings.TrimSpace(config.Description)
	if config.Description == "" {
		config.Description = DefaultAutoGroupDescription
	}
	return config
}

func GetAutoGroupConfig() AutoGroupConfig {
	autoGroupConfigMutex.RLock()
	defer autoGroupConfigMutex.RUnlock()
	return autoGroupConfig
}

func UpdateAutoGroupConfigByJsonString(jsonString string) error {
	var config AutoGroupConfig
	if err := common.UnmarshalJsonStr(jsonString, &config); err != nil {
		return err
	}
	autoGroupConfigMutex.Lock()
	autoGroupConfig = NormalizeAutoGroupConfig(config)
	autoGroupConfigMutex.Unlock()
	return nil
}

func AutoGroupConfig2JsonString() string {
	data, err := common.Marshal(GetAutoGroupConfig())
	if err != nil {
		return "{}"
	}
	return string(data)
}

var autoGroups = []string{
	"default",
}

var DefaultUseAutoGroup = false

var maxTokenAutoGroups atomic.Int64

func init() {
	maxTokenAutoGroups.Store(DefaultMaxTokenAutoGroups)
}

func ContainsAutoGroup(group string) bool {
	for _, autoGroup := range autoGroups {
		if autoGroup == group {
			return true
		}
	}
	return false
}

func UpdateAutoGroupsByJsonString(jsonString string) error {
	autoGroups = make([]string, 0)
	return common.Unmarshal([]byte(jsonString), &autoGroups)
}

func AutoGroups2JsonString() string {
	jsonBytes, err := common.Marshal(autoGroups)
	if err != nil {
		return "[]"
	}
	return string(jsonBytes)
}

func GetAutoGroups() []string {
	return autoGroups
}

func GetMaxTokenAutoGroups() int {
	return int(maxTokenAutoGroups.Load())
}

func ValidateMaxTokenAutoGroups(value string) error {
	maxCount, err := strconv.Atoi(value)
	if err != nil || maxCount <= 0 {
		return fmt.Errorf("MaxTokenAutoGroups must be a positive integer")
	}
	return nil
}

func UpdateMaxTokenAutoGroups(value string) error {
	if err := ValidateMaxTokenAutoGroups(value); err != nil {
		return err
	}
	maxCount, _ := strconv.Atoi(value)
	maxTokenAutoGroups.Store(int64(maxCount))
	return nil
}
