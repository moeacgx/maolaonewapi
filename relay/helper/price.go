package helper

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/priceformula"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func modelPriceNotConfiguredError(modelName string, userId int) error {
	if model.IsAdmin(userId) {
		return fmt.Errorf(
			"模型 %s 的价格未配置。请前往「系统设置 → 运营设置」开启自用模式，或在「系统设置 → 分组与模型定价设置」中为该模型配置价格；"+
				"Model %s price not configured. Go to System Settings → Operation Settings to enable self-use mode, or configure the model price in System Settings → Group & Model Pricing.",
			modelName, modelName,
		)
	}
	return fmt.Errorf(
		"模型 %s 的价格尚未由管理员配置，暂时无法使用，请联系站点管理员开启该模型；"+
			"Model %s has not been priced by the administrator yet. Please contact the site administrator to enable this model.",
		modelName, modelName,
	)
}

// https://docs.claude.com/en/docs/build-with-claude/prompt-caching#1-hour-cache-duration
const claudeCacheCreation1hMultiplier = 6 / 3.75

// 客户端未提供最大输出时，为分层计费预扣一个保守的输出上限。
const defaultTieredPreConsumeMaxTokens = 8192

// HandleGroupRatio checks for "auto_group" in the context and updates the group ratio and relayInfo.UsingGroup if present
func HandleGroupRatio(ctx *gin.Context, relayInfo *relaycommon.RelayInfo) types.GroupRatioInfo {
	groupRatioInfo := types.GroupRatioInfo{
		GroupRatio:        1.0, // default ratio
		GroupSpecialRatio: -1,
	}

	// check auto group
	autoGroup, exists := ctx.Get("auto_group")
	if exists {
		logger.LogDebug(ctx, "final group: %s", autoGroup)
		relayInfo.UsingGroup = autoGroup.(string)
	}

	// check user group special ratio
	userGroupRatio, ok := ratio_setting.GetGroupGroupRatio(relayInfo.UserGroup, relayInfo.UsingGroup)
	if ok {
		// user group special ratio
		groupRatioInfo.GroupSpecialRatio = userGroupRatio
		groupRatioInfo.GroupRatio = userGroupRatio
		groupRatioInfo.HasSpecialRatio = true
	} else {
		// normal group ratio
		groupRatioInfo.GroupRatio = ratio_setting.GetGroupRatio(relayInfo.UsingGroup)
	}

	return groupRatioInfo
}

func ModelPriceHelper(c *gin.Context, info *relaycommon.RelayInfo, promptTokens int, meta *types.TokenCountMeta) (types.PriceData, error) {
	modelPrice, usePrice := ratio_setting.GetModelPrice(info.OriginModelName, false)
	if !usePrice && hasRouteFormulaPrice(info) {
		modelPrice = 0
		usePrice = true
	}

	groupRatioInfo := HandleGroupRatio(c, info)

	// Check if this model uses tiered_expr billing
	if billing_setting.GetBillingMode(info.OriginModelName) == billing_setting.BillingModeTieredExpr {
		return modelPriceHelperTiered(c, info, promptTokens, meta, groupRatioInfo)
	}

	var preConsumedQuota int
	var modelRatio float64
	var completionRatio float64
	var cacheRatio float64
	var imageRatio float64
	var cacheCreationRatio float64
	var cacheCreationRatio5m float64
	var cacheCreationRatio1h float64
	var audioRatio float64
	var audioCompletionRatio float64
	var freeModel bool
	if !usePrice {
		preConsumedTokens := common.Max(promptTokens, common.PreConsumedQuota)
		if meta.MaxTokens != 0 {
			preConsumedTokens += meta.MaxTokens
		}
		var success bool
		var matchName string
		modelRatio, success, matchName = ratio_setting.GetModelRatio(info.OriginModelName)
		if !success {
			acceptUnsetRatio := false
			if info.UserSetting.AcceptUnsetRatioModel {
				acceptUnsetRatio = true
			}
			if !acceptUnsetRatio {
				return types.PriceData{}, modelPriceNotConfiguredError(matchName, info.UserId)
			}
		}
		completionRatio = ratio_setting.GetCompletionRatio(info.OriginModelName)
		cacheRatio, _ = ratio_setting.GetCacheRatio(info.OriginModelName)
		cacheCreationRatio, _ = ratio_setting.GetCreateCacheRatio(info.OriginModelName)
		cacheCreationRatio5m = cacheCreationRatio
		// 固定1h和5min缓存写入价格的比例
		cacheCreationRatio1h = cacheCreationRatio * claudeCacheCreation1hMultiplier
		imageRatio, _ = ratio_setting.GetImageRatio(info.OriginModelName)
		audioRatio = ratio_setting.GetAudioRatio(info.OriginModelName)
		audioCompletionRatio = ratio_setting.GetAudioCompletionRatio(info.OriginModelName)
		ratio := modelRatio * groupRatioInfo.GroupRatio
		quota, err := common.QuotaFromFloatStrict(float64(preConsumedTokens) * ratio)
		if err != nil {
			return types.PriceData{}, err
		}
		preConsumedQuota = quota
	} else {
		if meta.ImagePriceRatio != 0 {
			modelPrice = modelPrice * meta.ImagePriceRatio
		}
	}

	// check if free model pre-consume is disabled
	if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
		// if model price or ratio is 0, do not pre-consume quota
		if groupRatioInfo.GroupRatio == 0 {
			preConsumedQuota = 0
			freeModel = true
		} else if usePrice {
			if modelPrice == 0 {
				preConsumedQuota = 0
				freeModel = true
			}
		} else {
			if modelRatio == 0 {
				preConsumedQuota = 0
				freeModel = true
			}
		}
	}

	priceData := types.PriceData{
		FreeModel:            freeModel,
		ModelPrice:           modelPrice,
		ModelPriceUnit:       ratio_setting.GetModelPriceUnit(info.OriginModelName),
		ModelRatio:           modelRatio,
		CompletionRatio:      completionRatio,
		GroupRatioInfo:       groupRatioInfo,
		UsePrice:             usePrice,
		CacheRatio:           cacheRatio,
		ImageRatio:           imageRatio,
		AudioRatio:           audioRatio,
		AudioCompletionRatio: audioCompletionRatio,
		CacheCreationRatio:   cacheCreationRatio,
		CacheCreation5mRatio: cacheCreationRatio5m,
		CacheCreation1hRatio: cacheCreationRatio1h,
		QuotaToPreConsume:    preConsumedQuota,
	}
	if usePrice {
		for name, ratio := range meta.BillingRatios {
			priceData.AddOtherRatio(name, ratio)
		}
		if err := applyModelPriceVariantDimensions(&priceData, info, meta, promptTokens); err != nil {
			return types.PriceData{}, err
		}
		quotaToPreConsume := priceData.ApplyOtherRatiosToFloat(priceData.ModelPrice * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
		quota, err := common.QuotaFromFloatStrict(quotaToPreConsume)
		if err != nil {
			return types.PriceData{}, err
		}
		priceData.QuotaToPreConsume = quota
	}

	if common.DebugEnabled {
		logger.LogDebug(c, "model_price_helper result: %s", priceData.ToSetting())
	}
	info.PriceData = priceData
	return priceData, nil
}

func hasRouteFormulaPrice(info *relaycommon.RelayInfo) bool {
	route := imagePriceRouteFromRelayInfo(info)
	if route == "" || info == nil {
		return false
	}
	config, configured := ratio_setting.GetModelRoutePriceVariantConfig(info.OriginModelName, route)
	return configured && ratio_setting.HasEnabledModelPriceFormula(config)
}

func applyModelPriceVariantDimensions(priceData *types.PriceData, info *relaycommon.RelayInfo, meta *types.TokenCountMeta, promptTokens int) error {
	if priceData == nil || info == nil || meta == nil || !priceData.UsePrice {
		return nil
	}
	for key, value := range meta.BillingDimensions {
		priceData.AddBillingMeta(strings.ToLower(strings.TrimSpace(key)), strings.ToLower(strings.TrimSpace(value)))
	}
	modelConfig, modelConfigured := ratio_setting.GetModelPriceVariantConfig(info.OriginModelName)
	routeConfig, routeConfigured, routeMatched, err := applyModelRoutePriceVariantDimensions(priceData, info, meta, promptTokens)
	if err != nil {
		return err
	}
	if routeMatched {
		if priceData.BillingMeta["route_price_status"] != "formula" {
			applyModelPriceExtraParams(priceData, meta, modelConfig, modelConfigured, routeConfig, routeConfigured)
		}
		return nil
	}
	if !modelConfigured {
		applyModelPriceExtraParams(priceData, meta, modelConfig, modelConfigured, routeConfig, routeConfigured)
		return nil
	}
	match := ratio_setting.MatchModelPriceVariantConfig(modelConfig, meta.BillingDimensions)
	if !match.Matched {
		if modelConfig.ResolutionEnabled || modelConfig.QualityEnabled {
			// 缺档时保留旧价格/倍率，避免未知高规格静默回落到低价。
			priceData.AddBillingMeta("variant_price_status", "legacy")
		} else {
			priceData.AddBillingMeta("variant_price_status", "disabled")
		}
		applyModelPriceExtraParams(priceData, meta, modelConfig, modelConfigured, routeConfig, routeConfigured)
		return nil
	}
	priceData.ModelPrice = match.Price
	priceData.AddBillingMeta("variant_price_status", "matched")
	applyModelPriceExtraParams(priceData, meta, modelConfig, modelConfigured, routeConfig, routeConfigured)
	return nil
}

func imagePriceRouteFromRelayInfo(info *relaycommon.RelayInfo) string {
	if info == nil {
		return ""
	}
	if info.RelayMode == relayconstant.RelayModeImagesEdits {
		return ratio_setting.ModelPriceRouteImageEdit
	}
	return ""
}

func applyModelRoutePriceVariantDimensions(priceData *types.PriceData, info *relaycommon.RelayInfo, meta *types.TokenCountMeta, promptTokens int) (ratio_setting.ModelPriceVariantConfig, bool, bool, error) {
	route := imagePriceRouteFromRelayInfo(info)
	if route == "" {
		return ratio_setting.ModelPriceVariantConfig{}, false, false, nil
	}
	priceData.AddBillingMeta("price_route", route)
	config, configured := ratio_setting.GetModelRoutePriceVariantConfig(info.OriginModelName, route)
	if !configured {
		return ratio_setting.ModelPriceVariantConfig{}, false, false, nil
	}
	if ratio_setting.HasEnabledModelPriceFormula(config) {
		result, err := priceformula.Evaluate(priceformula.Config{
			Expression: config.Formula.Expression,
			Variables:  config.Formula.Variables,
			Defaults:   config.Formula.Defaults,
		}, priceformula.Input{
			BasePrice:             priceData.ModelPrice,
			Dimensions:            meta.BillingDimensions,
			Params:                meta.BillingParams,
			InputImages:           billingImagesForFormula(meta.BillingImages),
			EstimatedPromptTokens: promptTokens,
			PromptChars:           len(meta.CombineText),
		})
		if err != nil {
			return config, true, false, err
		}
		priceData.ModelPrice = result.Price
		priceData.AddBillingMeta("mode", "route_formula")
		priceData.AddBillingMeta("route_price_status", "formula")
		priceData.AddBillingMeta("formula_price", formatBillingFloat(result.Price))
		if result.Quality != "" {
			priceData.AddBillingMeta("formula_quality", result.Quality)
		}
		for _, key := range []string{"width", "height", "input_images", "input_image_count", "prompt_tokens_estimated", "prompt_chars"} {
			if value, ok := result.Variables[key]; ok {
				priceData.AddBillingMeta("formula_"+key, formatBillingFloat(value))
			}
		}
		for _, key := range sortedFormulaKeys(config.Formula.Variables) {
			priceData.AddBillingMeta("formula_var_"+key, formatBillingFloat(config.Formula.Variables[key]))
		}
		for _, key := range sortedFormulaStringKeys(config.Formula.Defaults) {
			priceData.AddBillingMeta("formula_default_"+key, config.Formula.Defaults[key])
		}
		priceData.AddBillingMeta("formula_detail", buildRouteFormulaBillingDetail(config.Formula, result))
		return config, true, true, nil
	}
	match := ratio_setting.MatchModelRoutePriceVariant(info.OriginModelName, route, meta.BillingDimensions)
	if !match.Matched {
		if config.ResolutionEnabled || config.QualityEnabled {
			priceData.AddBillingMeta("route_price_status", "legacy")
		} else {
			priceData.AddBillingMeta("route_price_status", "disabled")
		}
		return config, true, false, nil
	}
	priceData.ModelPrice = match.Price
	priceData.AddBillingMeta("route_price_status", "matched")
	return config, true, true, nil
}

func billingImagesForFormula(images []types.BillingImageMeta) []priceformula.ImageDimension {
	if len(images) == 0 {
		return nil
	}
	result := make([]priceformula.ImageDimension, 0, len(images))
	for _, image := range images {
		if image.Width <= 0 || image.Height <= 0 {
			continue
		}
		result = append(result, priceformula.ImageDimension{Width: image.Width, Height: image.Height})
	}
	return result
}

func applyModelPriceExtraParams(priceData *types.PriceData, meta *types.TokenCountMeta, modelConfig ratio_setting.ModelPriceVariantConfig, modelConfigured bool, routeConfig ratio_setting.ModelPriceVariantConfig, routeConfigured bool) {
	if priceData == nil || meta == nil || len(meta.BillingParams) == 0 {
		return
	}
	effectiveConfig := modelConfig
	effectiveConfigured := modelConfigured
	if routeConfigured && len(routeConfig.ExtraParams) > 0 {
		effectiveConfig = routeConfig
		effectiveConfigured = true
	}
	if !effectiveConfigured || len(effectiveConfig.ExtraParams) == 0 {
		return
	}
	charges := ratio_setting.CalculateModelPriceExtraParamCharges(effectiveConfig, meta.BillingParams)
	if len(charges) == 0 {
		return
	}
	var total float64
	for _, charge := range charges {
		total += charge.Price
		key := strings.ToLower(strings.TrimSpace(charge.Key))
		if key == "" {
			continue
		}
		priceData.AddBillingMeta("extra_param_"+key, formatBillingFloat(charge.Value))
		priceData.AddBillingMeta("extra_param_"+key+"_base", formatBillingFloat(charge.Base))
		priceData.AddBillingMeta("extra_param_"+key+"_unit_price", formatBillingFloat(charge.UnitPrice))
		priceData.AddBillingMeta("extra_param_"+key+"_extra_units", formatBillingFloat(charge.ExtraUnits))
		priceData.AddBillingMeta("extra_param_"+key+"_price", formatBillingFloat(charge.Price))
	}
	if total <= 0 {
		return
	}
	priceData.ModelPrice += total
	priceData.AddBillingMeta("extra_price", formatBillingFloat(total))
}

func formatBillingFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func buildRouteFormulaBillingDetail(formula *ratio_setting.ModelPriceFormulaConfig, result priceformula.Result) string {
	if formula == nil {
		return ""
	}
	parts := []string{"公式计费：按照上游公式计算"}
	if result.Quality != "" {
		parts = append(parts, fmt.Sprintf("品质 %s", result.Quality))
	}
	if width, ok := result.Variables["width"]; ok && width > 0 {
		height := result.Variables["height"]
		parts = append(parts, fmt.Sprintf("输出规格 %.0fx%.0f", width, height))
	}
	if inputImages, ok := result.Variables["input_images"]; ok && inputImages > 0 {
		parts = append(parts, fmt.Sprintf("输入图片 %.0f 张", inputImages))
	} else if inputImages, ok := result.Variables["input_image_count"]; ok && inputImages > 0 {
		parts = append(parts, fmt.Sprintf("输入图片 %.0f 张", inputImages))
	}
	if promptChars, ok := result.Variables["prompt_chars"]; ok && promptChars > 0 {
		parts = append(parts, fmt.Sprintf("提示词长度 %.0f 字", promptChars))
	}
	if len(formula.Variables) > 0 {
		keys := sortedFormulaKeys(formula.Variables)
		vars := make([]string, 0, len(keys))
		for _, key := range keys {
			vars = append(vars, fmt.Sprintf("%s=%s", key, formatBillingFloat(formula.Variables[key])))
		}
		parts = append(parts, "公式变量 "+strings.Join(vars, ", "))
	}
	if len(formula.Defaults) > 0 {
		keys := sortedFormulaStringKeys(formula.Defaults)
		defs := make([]string, 0, len(keys))
		for _, key := range keys {
			defs = append(defs, fmt.Sprintf("%s=%s", key, formula.Defaults[key]))
		}
		parts = append(parts, "默认值 "+strings.Join(defs, ", "))
	}
	return strings.Join(parts, "；")
}

func sortedFormulaKeys(values map[string]float64) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedFormulaStringKeys(values map[string]string) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// ModelPriceHelperPerCall 固定单价/倍率任务的 PriceHelper（MJ、Task）。
// 固定单价的具体单位由 PriceData.ModelPriceUnit 决定。
func ModelPriceHelperPerCall(c *gin.Context, info *relaycommon.RelayInfo) (types.PriceData, error) {
	groupRatioInfo := HandleGroupRatio(c, info)

	modelPrice, success := ratio_setting.GetModelPrice(info.OriginModelName, true)
	usePrice := success
	var modelRatio float64

	if !success {
		defaultPrice, ok := ratio_setting.GetDefaultModelPriceMap()[info.OriginModelName]
		if ok {
			modelPrice = defaultPrice
			usePrice = true
		} else {
			var ratioSuccess bool
			var matchName string
			modelRatio, ratioSuccess, matchName = ratio_setting.GetModelRatio(info.OriginModelName)
			acceptUnsetRatio := false
			if info.UserSetting.AcceptUnsetRatioModel {
				acceptUnsetRatio = true
			}
			if !ratioSuccess && !acceptUnsetRatio {
				return types.PriceData{}, modelPriceNotConfiguredError(matchName, info.UserId)
			}
		}
	}

	var quota int
	freeModel := false

	if usePrice {
		var err error
		quota, err = common.QuotaFromFloatStrict(modelPrice * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
		if err != nil {
			return types.PriceData{}, err
		}
		if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
			if groupRatioInfo.GroupRatio == 0 || modelPrice == 0 {
				quota = 0
				freeModel = true
			}
		}
	} else {
		// 按量计费：以模型倍率的一半作为预扣额度
		var err error
		quota, err = common.QuotaFromFloatStrict(modelRatio / 2 * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
		if err != nil {
			return types.PriceData{}, err
		}
		modelPrice = -1
		if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
			if groupRatioInfo.GroupRatio == 0 || modelRatio == 0 {
				quota = 0
				freeModel = true
			}
		}
	}

	priceData := types.PriceData{
		FreeModel:      freeModel,
		ModelPrice:     modelPrice,
		ModelPriceUnit: ratio_setting.GetModelPriceUnit(info.OriginModelName),
		ModelRatio:     modelRatio,
		UsePrice:       usePrice,
		Quota:          quota,
		GroupRatioInfo: groupRatioInfo,
	}
	return priceData, nil
}

func HasModelBillingConfig(modelName string) bool {
	if _, ok := ratio_setting.GetModelPrice(modelName, false); ok {
		return true
	}
	if _, ok, _ := ratio_setting.GetModelRatio(modelName); ok {
		return true
	}
	if billing_setting.GetBillingMode(modelName) != billing_setting.BillingModeTieredExpr {
		return false
	}
	expr, ok := billing_setting.GetBillingExpr(modelName)
	return ok && strings.TrimSpace(expr) != ""
}

func modelPriceHelperTiered(c *gin.Context, info *relaycommon.RelayInfo, promptTokens int, meta *types.TokenCountMeta, groupRatioInfo types.GroupRatioInfo) (types.PriceData, error) {
	exprStr, ok := billing_setting.GetBillingExpr(info.OriginModelName)
	if !ok {
		return types.PriceData{}, fmt.Errorf("model %s is configured as tiered_expr but has no billing expression", info.OriginModelName)
	}

	estimatedCompletionTokens := meta.MaxTokens
	if estimatedCompletionTokens == 0 && groupRatioInfo.GroupRatio != 0 {
		estimatedCompletionTokens = defaultTieredPreConsumeMaxTokens
	}

	requestInput, err := ResolveIncomingBillingExprRequestInput(c, info)
	if err != nil {
		return types.PriceData{}, err
	}

	rawCost, trace, err := billingexpr.RunExprWithRequest(exprStr, billingexpr.TokenParams{
		P:   float64(promptTokens),
		C:   float64(estimatedCompletionTokens),
		Len: float64(promptTokens),
	}, requestInput)
	if err != nil {
		return types.PriceData{}, fmt.Errorf("model %s tiered expr run failed: %w", info.OriginModelName, err)
	}

	// Expression coefficients are $/1M tokens prices; convert to quota the same way per-call billing does.
	quotaBeforeGroup := rawCost / 1_000_000 * common.QuotaPerUnit
	preConsumedQuota, err := billingexpr.QuotaRoundStrict(quotaBeforeGroup * groupRatioInfo.GroupRatio)
	if err != nil {
		return types.PriceData{}, err
	}

	freeModel := false
	if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
		if groupRatioInfo.GroupRatio == 0 {
			preConsumedQuota = 0
			freeModel = true
		}
	}

	exprHash := billingexpr.ExprHashString(exprStr)
	snapshot := &billingexpr.BillingSnapshot{
		BillingMode:               billing_setting.BillingModeTieredExpr,
		ModelName:                 info.OriginModelName,
		ExprString:                exprStr,
		ExprHash:                  exprHash,
		GroupRatio:                groupRatioInfo.GroupRatio,
		EstimatedPromptTokens:     promptTokens,
		EstimatedCompletionTokens: estimatedCompletionTokens,
		EstimatedQuotaBeforeGroup: quotaBeforeGroup,
		EstimatedQuotaAfterGroup:  preConsumedQuota,
		EstimatedTier:             trace.MatchedTier,
		QuotaPerUnit:              common.QuotaPerUnit,
		ExprVersion:               billingexpr.ExprVersion(exprStr),
	}
	info.TieredBillingSnapshot = snapshot
	info.BillingRequestInput = &requestInput

	priceData := types.PriceData{
		FreeModel:         freeModel,
		GroupRatioInfo:    groupRatioInfo,
		QuotaToPreConsume: preConsumedQuota,
	}

	logger.LogDebug(c, "model_price_helper_tiered result: model=%s preConsume=%d quotaBeforeGroup=%.2f groupRatio=%.2f tier=%s", info.OriginModelName, preConsumedQuota, quotaBeforeGroup, groupRatioInfo.GroupRatio, trace.MatchedTier)

	info.PriceData = priceData
	return priceData, nil
}
