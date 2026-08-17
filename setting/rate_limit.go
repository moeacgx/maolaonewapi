package setting

import (
	"fmt"
	"math"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
)

type RateLimitCounts [2]int

type UserGroupRateLimit struct {
	Global *RateLimitCounts           `json:"global,omitempty"`
	Groups map[string]RateLimitCounts `json:"groups,omitempty"`
}

// ModelRequestRateLimitSnapshot is an immutable generation of all model request
// rate-limit settings. Retain it for operations that require a coherent view.
type ModelRequestRateLimitSnapshot struct {
	enabled         bool
	durationMinutes int
	global          RateLimitCounts
	groups          map[string]RateLimitCounts
	userGroups      map[string]UserGroupRateLimit
}

var (
	modelRequestRateLimitSnapshot atomic.Pointer[ModelRequestRateLimitSnapshot]
	modelRequestRateLimitUpdateMu sync.Mutex
)

func init() {
	modelRequestRateLimitSnapshot.Store(&ModelRequestRateLimitSnapshot{
		durationMinutes: 1,
		global:          RateLimitCounts{0, 1000},
		groups:          map[string]RateLimitCounts{},
		userGroups:      map[string]UserGroupRateLimit{},
	})
}

func GetModelRequestRateLimitSnapshot() *ModelRequestRateLimitSnapshot {
	return modelRequestRateLimitSnapshot.Load()
}

func (s *ModelRequestRateLimitSnapshot) Enabled() bool {
	return s.enabled
}

func (s *ModelRequestRateLimitSnapshot) DurationMinutes() int {
	return s.durationMinutes
}

func (s *ModelRequestRateLimitSnapshot) GlobalRateLimit() (totalCount, successCount int) {
	return s.global[0], s.global[1]
}

func (s *ModelRequestRateLimitSnapshot) GetGroupRateLimit(group string) (totalCount, successCount int, found bool) {
	limits, found := s.groups[group]
	if !found {
		return 0, 0, false
	}
	return limits[0], limits[1], true
}

func (s *ModelRequestRateLimitSnapshot) GetUserGroupGlobalRateLimit(userGroup string) (totalCount, successCount int, found bool) {
	limits, found := s.userGroups[userGroup]
	if !found || limits.Global == nil {
		return 0, 0, false
	}
	return (*limits.Global)[0], (*limits.Global)[1], true
}

func (s *ModelRequestRateLimitSnapshot) GetUserGroupRateLimit(userGroup, group string) (totalCount, successCount int, found bool) {
	limits, found := s.userGroups[userGroup]
	if !found {
		return 0, 0, false
	}
	groupLimits, found := limits.Groups[group]
	if !found {
		return 0, 0, false
	}
	return groupLimits[0], groupLimits[1], true
}

func (s *ModelRequestRateLimitSnapshot) GroupJSONString() string {
	jsonBytes, err := common.Marshal(s.groups)
	if err != nil {
		common.SysLog("error marshalling model request rate limit group: " + err.Error())
	}
	return string(jsonBytes)
}

func (s *ModelRequestRateLimitSnapshot) UserGroupJSONString() string {
	jsonBytes, err := common.Marshal(s.userGroups)
	if err != nil {
		common.SysLog("error marshalling model request rate limit user group: " + err.Error())
	}
	return string(jsonBytes)
}

func ModelRequestRateLimitGroup2JSONString() string {
	return GetModelRequestRateLimitSnapshot().GroupJSONString()
}

func ModelRequestRateLimitUserGroup2JSONString() string {
	return GetModelRequestRateLimitSnapshot().UserGroupJSONString()
}

// ValidateModelRequestRateLimitOptions parses and validates a partial option
// update against the current generation without publishing it.
func ValidateModelRequestRateLimitOptions(values map[string]string) error {
	modelRequestRateLimitUpdateMu.Lock()
	defer modelRequestRateLimitUpdateMu.Unlock()
	_, err := buildModelRequestRateLimitSnapshot(GetModelRequestRateLimitSnapshot(), values)
	return err
}

// UpdateModelRequestRateLimitOptions publishes all supplied model rate-limit
// options as one generation. No part is visible on parse or validation failure.
func UpdateModelRequestRateLimitOptions(values map[string]string) error {
	modelRequestRateLimitUpdateMu.Lock()
	defer modelRequestRateLimitUpdateMu.Unlock()
	next, err := buildModelRequestRateLimitSnapshot(GetModelRequestRateLimitSnapshot(), values)
	if err != nil {
		return err
	}
	modelRequestRateLimitSnapshot.Store(next)
	return nil
}

func buildModelRequestRateLimitSnapshot(current *ModelRequestRateLimitSnapshot, values map[string]string) (*ModelRequestRateLimitSnapshot, error) {
	next := &ModelRequestRateLimitSnapshot{
		enabled:         current.enabled,
		durationMinutes: current.durationMinutes,
		global:          current.global,
		groups:          current.groups,
		userGroups:      current.userGroups,
	}
	for key, value := range values {
		switch key {
		case "ModelRequestRateLimitEnabled":
			if value != "true" && value != "false" {
				return nil, fmt.Errorf("%s must be true or false", key)
			}
			next.enabled = value == "true"
		case "ModelRequestRateLimitDurationMinutes":
			parsed, err := parseModelRequestRateLimitInt(key, value, 1)
			if err != nil {
				return nil, err
			}
			next.durationMinutes = parsed
		case "ModelRequestRateLimitCount":
			parsed, err := parseModelRequestRateLimitInt(key, value, 0)
			if err != nil {
				return nil, err
			}
			next.global[0] = parsed
		case "ModelRequestRateLimitSuccessCount":
			parsed, err := parseModelRequestRateLimitInt(key, value, 1)
			if err != nil {
				return nil, err
			}
			next.global[1] = parsed
		case "ModelRequestRateLimitGroup":
			groups := make(map[string]RateLimitCounts)
			if err := common.UnmarshalJsonStr(value, &groups); err != nil {
				return nil, err
			}
			if err := validateModelRequestRateLimitGroup(groups); err != nil {
				return nil, err
			}
			next.groups = groups
		case "ModelRequestRateLimitUserGroup":
			userGroups := make(map[string]UserGroupRateLimit)
			if err := common.UnmarshalJsonStr(value, &userGroups); err != nil {
				return nil, err
			}
			if err := validateModelRequestRateLimitUserGroup(userGroups); err != nil {
				return nil, err
			}
			next.userGroups = userGroups
		default:
			return nil, fmt.Errorf("unknown model request rate limit option %s", key)
		}
	}
	if err := validateRateLimitCounts("global", next.global); err != nil {
		return nil, err
	}
	return next, nil
}

func parseModelRequestRateLimitInt(key, value string, minimum int) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	if parsed < minimum || parsed > math.MaxInt32 {
		return 0, fmt.Errorf("%s must be between %d and %d", key, minimum, math.MaxInt32)
	}
	return parsed, nil
}

func UpdateModelRequestRateLimitGroupByJSONString(jsonStr string) error {
	return UpdateModelRequestRateLimitOptions(map[string]string{"ModelRequestRateLimitGroup": jsonStr})
}

func UpdateModelRequestRateLimitUserGroupByJSONString(jsonStr string) error {
	return UpdateModelRequestRateLimitOptions(map[string]string{"ModelRequestRateLimitUserGroup": jsonStr})
}

func GetGroupRateLimit(group string) (totalCount, successCount int, found bool) {
	return GetModelRequestRateLimitSnapshot().GetGroupRateLimit(group)
}

func GetUserGroupGlobalRateLimit(userGroup string) (totalCount, successCount int, found bool) {
	return GetModelRequestRateLimitSnapshot().GetUserGroupGlobalRateLimit(userGroup)
}

func GetUserGroupRateLimit(userGroup, group string) (totalCount, successCount int, found bool) {
	return GetModelRequestRateLimitSnapshot().GetUserGroupRateLimit(userGroup, group)
}

func CheckModelRequestRateLimitGroup(jsonStr string) error {
	return ValidateModelRequestRateLimitOptions(map[string]string{"ModelRequestRateLimitGroup": jsonStr})
}

func validateModelRequestRateLimitGroup(values map[string]RateLimitCounts) error {
	for group, limits := range values {
		if err := validateRateLimitCounts(fmt.Sprintf("group %s", group), limits); err != nil {
			return err
		}
	}
	return nil
}

func CheckModelRequestRateLimitUserGroup(jsonStr string) error {
	return ValidateModelRequestRateLimitOptions(map[string]string{"ModelRequestRateLimitUserGroup": jsonStr})
}

func validateModelRequestRateLimitUserGroup(values map[string]UserGroupRateLimit) error {
	for userGroup, limits := range values {
		if userGroup == "" {
			return fmt.Errorf("user group is empty")
		}
		if limits.Global != nil {
			if err := validateRateLimitCounts(fmt.Sprintf("user group %s global", userGroup), *limits.Global); err != nil {
				return err
			}
		}
		for group, groupLimits := range limits.Groups {
			if group == "" {
				return fmt.Errorf("user group %s request group is empty", userGroup)
			}
			if err := validateRateLimitCounts(fmt.Sprintf("user group %s request group %s", userGroup, group), groupLimits); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateRateLimitCounts(label string, limits RateLimitCounts) error {
	if limits[0] < 0 {
		return fmt.Errorf("%s has negative rate limit total value: [%d, %d]", label, limits[0], limits[1])
	}
	if limits[1] < 1 {
		return fmt.Errorf("%s has invalid success rate limit value: [%d, %d]", label, limits[0], limits[1])
	}
	if limits[0] > math.MaxInt32 || limits[1] > math.MaxInt32 {
		return fmt.Errorf("%s [%d, %d] has max rate limits value 2147483647", label, limits[0], limits[1])
	}
	return nil
}
