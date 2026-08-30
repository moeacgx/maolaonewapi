var sdk = globalThis.__NEW_API_EXTENSION_NATIVE_SDK__;
if (!sdk || sdk.sdk !== "v1" || sdk.platform !== "classic") {
  throw new Error("Native extension SDK v1 for classic is unavailable.");
}
var react = sdk.modules.react;
var runtime = sdk.modules["react/jsx-runtime"];
var helperModule = sdk.modules["../../helpers"];
if (!react || !runtime || !helperModule) {
  throw new Error("OKX Alipay rate module host SDK is incomplete.");
}

var useEffect = react.useEffect;
var useState = react.useState;
var jsx = runtime.jsx;
var jsxs = runtime.jsxs;
var API = helperModule.API;

var defaultConfig = {
  rate_api_url: "",
  side: "buy",
  tier: 3,
  adjustment_type: "absolute",
  adjustment_value: "0",
};

function text(value) {
  if (value === undefined || value === null || value === "") return "--";
  return String(value);
}

function normalizeConfig(value) {
  return {
    rate_api_url: text(value?.rate_api_url) === "--" ? "" : String(value.rate_api_url),
    side: value?.side === "sell" ? "sell" : "buy",
    tier: Number.parseInt(String(value?.tier || defaultConfig.tier), 10) || defaultConfig.tier,
    adjustment_type: value?.adjustment_type === "percent" ? "percent" : "absolute",
    adjustment_value: text(value?.adjustment_value) === "--" ? "0" : String(value.adjustment_value),
  };
}

function configPayload(config) {
  return {
    rate_api_url: String(config.rate_api_url || "").trim(),
    side: config.side === "sell" ? "sell" : "buy",
    tier: Number.parseInt(String(config.tier || defaultConfig.tier), 10),
    adjustment_type: config.adjustment_type === "percent" ? "percent" : "absolute",
    adjustment_value: Number.parseFloat(String(config.adjustment_value || "0")),
  };
}

function rateText(value) {
  return value ? value + " CNY/USDT" : "--";
}

function apiData(response) {
  if (!response?.data?.success) {
    throw new Error(response?.data?.message || "请求失败");
  }
  return response.data.data || {};
}

function Field(props) {
  return jsxs("label", {
    className: "okx-rate-field",
    children: [
      jsx("span", { children: props.label }),
      props.children,
      props.hint ? jsx("small", { children: props.hint }) : null,
    ],
  });
}

function Metric(props) {
  return jsxs("div", {
    className: "okx-rate-metric",
    children: [
      jsx("span", { children: props.label }),
      jsx("strong", { children: text(props.value) }),
    ],
  });
}

function OkxAlipayRateModule() {
  var state = useState(normalizeConfig(defaultConfig));
  var config = state[0];
  var setConfig = state[1];
  var quoteState = useState(null);
  var quote = quoteState[0];
  var setQuote = quoteState[1];
  var statusState = useState({ type: "loading", text: "正在加载模块配置..." });
  var status = statusState[0];
  var setStatus = statusState[1];
  var savingState = useState(false);
  var saving = savingState[0];
  var setSaving = savingState[1];
  var refreshingState = useState(false);
  var refreshing = refreshingState[0];
  var setRefreshing = refreshingState[1];

  function patchConfig(key, value) {
    setConfig(function (current) {
      var next = {};
      Object.assign(next, current);
      next[key] = value;
      return next;
    });
  }

  async function loadConfig() {
    setStatus({ type: "loading", text: "正在加载模块配置..." });
    var response = await API.get("/api/extension-admin/okx-alipay-rate/config", {
      skipErrorHandler: true,
    });
    var nextConfig = normalizeConfig(apiData(response));
    setConfig(nextConfig);
    return nextConfig;
  }

  async function refreshQuote() {
    setRefreshing(true);
    setStatus({ type: "loading", text: "正在读取 OKX 支付宝档位价格..." });
    try {
      var response = await API.get("/api/extension-admin/okx-alipay-rate/quote", {
        skipErrorHandler: true,
      });
      setQuote(apiData(response));
      setStatus({ type: "ok", text: "OKX 支付宝汇率已更新。" });
    } catch (error) {
      setStatus({ type: "error", text: error?.message || "获取 OKX 汇率失败" });
    } finally {
      setRefreshing(false);
    }
  }

  async function saveConfig() {
    setSaving(true);
    setStatus({ type: "loading", text: "正在保存模块配置..." });
    try {
      var response = await API.put(
        "/api/extension-admin/okx-alipay-rate/config",
        configPayload(config),
        { skipErrorHandler: true }
      );
      setConfig(normalizeConfig(apiData(response)));
      setStatus({ type: "ok", text: "配置已保存，正在刷新 OKX 汇率..." });
      await refreshQuote();
    } catch (error) {
      setStatus({ type: "error", text: error?.message || "保存配置失败" });
    } finally {
      setSaving(false);
    }
  }

  useEffect(function () {
    var active = true;
    loadConfig()
      .then(function () {
        if (active) return refreshQuote();
      })
      .catch(function (error) {
        if (active) {
          setStatus({ type: "error", text: error?.message || "加载配置失败" });
        }
      });
    return function () {
      active = false;
    };
  }, []);

  return jsxs("div", {
    className: "okx-rate-native okx-rate-classic",
    children: [
      jsxs("div", {
        className: "okx-rate-header",
        children: [
          jsxs("div", {
            children: [
              jsx("h1", { children: "OKX 支付宝汇率" }),
              jsx("p", {
                children: "读取 OKX C2C 支付宝档位价格，并按配置上浮或下浮后提供给 OKPay。",
              }),
            ],
          }),
          jsxs("div", {
            className: "okx-rate-actions",
            children: [
              jsx("button", {
                type: "button",
                disabled: saving,
                onClick: saveConfig,
                children: saving ? "保存中..." : "保存配置",
              }),
              jsx("button", {
                type: "button",
                className: "secondary",
                disabled: refreshing,
                onClick: refreshQuote,
                children: refreshing ? "刷新中..." : "刷新价格",
              }),
            ],
          }),
        ],
      }),
      jsx("div", {
        className: "okx-rate-status " + status.type,
        children: status.text,
      }),
      jsxs("div", {
        className: "okx-rate-grid",
        children: [
          jsx(Metric, { label: "OKPay 实际汇率", value: rateText(quote?.adjusted_rate) }),
          jsx(Metric, { label: "OKX 原始价格", value: rateText(quote?.raw_rate) }),
          jsx(Metric, { label: "来源", value: quote?.source || "okx-alipay-rate-module" }),
        ],
      }),
      jsxs("div", {
        className: "okx-rate-form",
        children: [
          jsx("h2", { children: "模块配置" }),
          jsxs("div", {
            className: "okx-rate-form-grid",
            children: [
              jsx(Field, {
                label: "OKX 接口地址",
                hint: "留空使用内置 OKX C2C 支付宝接口。",
                children: jsx("input", {
                  type: "url",
                  value: config.rate_api_url,
                  placeholder: "留空使用内置接口",
                  onChange: function (event) { patchConfig("rate_api_url", event.target.value); },
                }),
              }),
              jsx(Field, {
                label: "方向",
                hint: "OKPay 收款通常使用 buy 档位。",
                children: jsxs("select", {
                  value: config.side,
                  onChange: function (event) { patchConfig("side", event.target.value); },
                  children: [
                    jsx("option", { value: "buy", children: "收款档位 buy" }),
                    jsx("option", { value: "sell", children: "付款档位 sell" }),
                  ],
                }),
              }),
              jsx(Field, {
                label: "档位",
                hint: "例如 3 表示读取 OKX 返回列表第三档。",
                children: jsx("input", {
                  type: "number",
                  min: "1",
                  step: "1",
                  value: config.tier,
                  onChange: function (event) { patchConfig("tier", event.target.value); },
                }),
              }),
              jsx(Field, {
                label: "调价方式",
                hint: "固定偏移填 -0.2 表示 6.8 调成 6.6。",
                children: jsxs("select", {
                  value: config.adjustment_type,
                  onChange: function (event) { patchConfig("adjustment_type", event.target.value); },
                  children: [
                    jsx("option", { value: "absolute", children: "固定偏移" }),
                    jsx("option", { value: "percent", children: "百分比" }),
                  ],
                }),
              }),
              jsx(Field, {
                label: "调价值",
                hint: "可填正数上浮，也可填负数下浮。",
                children: jsx("input", {
                  type: "number",
                  step: "0.0001",
                  value: config.adjustment_value,
                  onChange: function (event) { patchConfig("adjustment_value", event.target.value); },
                }),
              }),
              jsx(Field, {
                label: "OKPay 接入方式",
                hint: "系统设置 - 支付设置 - OKPay 中选择 OKX 支付宝汇率模块。",
                children: jsx("div", {
                  className: "okx-rate-code",
                  children: "okx-alipay-rate-module",
                }),
              }),
            ],
          }),
        ],
      }),
      jsxs("div", {
        className: "okx-rate-summary",
        children: [
          jsxs("span", { children: ["档位：", jsx("code", { children: text(quote?.tier) })] }),
          jsxs("span", { children: ["方向：", jsx("code", { children: text(quote?.side) })] }),
          jsxs("span", {
            children: [
              "调价：",
              jsx("code", {
                children: text((quote?.adjustment_type || "absolute") + " " + (quote?.adjustment_value || 0)),
              }),
            ],
          }),
          jsxs("span", { children: ["商户：", jsx("code", { children: text(quote?.nick_name) })] }),
        ],
      }),
    ],
  });
}

export default OkxAlipayRateModule;
