export type GroupIconOption = { value: string; label: string }

export const GROUP_ICON_OPTIONS: GroupIconOption[] = [
  ['OpenAI', 'OpenAI'], ['Claude.Color', 'Anthropic'], ['Gemini.Color', '谷歌 Gemini'],
  ['Qwen.Color', '阿里通义'], ['DeepSeek.Color', 'DeepSeek'], ['Zhipu.Color', '智谱'],
  ['Moonshot', '月之暗面 Kimi'], ['Minimax.Color', 'MiniMax'], ['Wenxin.Color', '百度'],
  ['Spark.Color', '讯飞'], ['Hunyuan.Color', '腾讯混元'], ['Doubao.Color', '豆包'],
  ['Yi.Color', '零一万物'], ['Kling.Color', '可灵'], ['Jimeng.Color', '即梦'],
  ['Vidu.Color', 'Vidu'], ['Mistral.Color', 'Mistral'], ['Cohere.Color', 'Cohere'],
  ['Cloudflare.Color', 'Cloudflare'], ['XAI', 'xAI Grok'], ['Ollama', 'Meta / Llama'],
  ['OpenRouter', 'OpenRouter'], ['Perplexity', 'Perplexity'], ['SiliconCloud', 'SiliconFlow'],
  ['Baidu', '百度'], ['AzureAI', 'Microsoft Azure'], ['Aws', 'AWS'], ['Coze', 'Coze'],
  ['Jina', 'Jina'], ['Suno', 'Suno'], ['Replicate', 'Replicate'], ['Midjourney', 'Midjourney'],
  ['Dify', 'Dify'], ['FastGPT', 'FastGPT'], ['Layers', '通用分组'],
].map(([value, label]) => ({ value, label }))

export function filterGroupIconOptions(query: string): GroupIconOption[] {
  const normalized = query.trim().toLowerCase()
  if (!normalized) return GROUP_ICON_OPTIONS
  return GROUP_ICON_OPTIONS.filter((option) =>
    `${option.value} ${option.label}`.toLowerCase().includes(normalized)
  )
}
