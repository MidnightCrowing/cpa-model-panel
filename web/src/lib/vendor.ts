/** Vendor detection lives in the frontend so the list can be tuned without a
 * backend deploy. Order matters: the first match wins. */
const RULES: Array<[string, RegExp]> = [
  ['Anthropic', /claude|anthropic/],
  ['OpenAI', /gpt|openai|^o[1-9](-|$)|davinci|dall-e|whisper|sora/],
  ['Google', /gemini|gemma|google|imagen|veo/],
  ['DeepSeek', /deepseek/],
  ['阿里巴巴', /qwen|qwq|tongyi|aliyun|alibaba|wanx/],
  ['Moonshot', /moonshot|kimi/],
  ['智谱', /glm|chatglm|zhipu|cogview|智谱/],
  ['字节跳动', /doubao|seed-|seedream|seededit|volcengine|字节/],
  ['xAI', /grok|x-ai|xai/],
  ['Meta', /llama|meta-/],
  ['Mistral', /mistral|magistral|codestral|devstral|pixtral/],
  ['MiniMax', /minimax|abab/],
  ['腾讯', /hunyuan|tencent|腾讯/],
  ['讯飞', /spark|iflytek|xunfei/],
  ['百度', /ernie|wenxin|baidu|百度/],
  ['零一万物', /01-ai|(^|[-/])yi-/],
  ['Cohere', /command-|cohere|rerank-(english|multilingual)/],
  ['Jina', /jina/],
  ['Cloudflare', /@cf\/|cloudflare|^cf\//],
  ['即梦', /jimeng|即梦/],
  ['BAAI', /bge-|baai/],
  ['Stability', /stable-diffusion|stabilityai|sdxl/],
  ['NVIDIA', /nvidia|nemotron/],
  ['Microsoft', /phi-|microsoft/],
]

/** Every vendor the rules can produce, in display order. */
export const VENDORS = RULES.map(([name]) => name)

export const UNKNOWN_VENDOR = '其他'

export function vendorOf(...names: string[]): string {
  for (const name of names) {
    if (!name) continue
    const value = name.toLowerCase()
    for (const [vendor, pattern] of RULES) {
      if (pattern.test(value)) return vendor
    }
  }
  return UNKNOWN_VENDOR
}
