package setting

var (
	OkpayGatewayUrl          = "https://api.okaypay.me/shop"
	OkpayMerchantId          string
	OkpayMerchantToken       string
	OkpayExchangeRate        float64 = 7.2
	OkpayAutoExchangeEnabled bool    = true
	OkpayUsdtCnyRate         float64 = 7.2
	OkpayRateApiUrl                  = "https://api.coingecko.com/api/v3/simple/price?ids=tether&vs_currencies=cny&include_last_updated_at=true"
	OkpayRateSource                  = "coingecko"
	OkpayOkxSide                     = "buy"
	OkpayOkxTier             int     = 3
	OkpayRateAdjustmentType          = "absolute"
	OkpayRateAdjustmentValue float64
	OkpayMinTopUp            int = 1
	OkpayCoin                    = "USDT"
)
