设计这样一个系统（我们暂且称之为 **NexusRouter**），我们需要将其拆分为数据面（Data Plane，负责高吞吐流量转发）和控制面（Control Plane，负责账户、计费、路由规则）。

以下是 NexusRouter 的整体系统建设方案。

---

## 一、 系统核心定位与能力边界

### 1. 基础对标能力
* **统一 API 标准**：以 OpenAI API 格式作为“世界语”，兼容所有主流模型（OpenAI, Anthropic, Google, Meta 等）。
* **高速透传与适配**：支持原生结构的直接透传，也支持跨格式的平滑转换（如 OpenAI 格式转为 Anthropic 的 Messages API）。
* **流式输出 (Streaming)**：全面支持 Server-Sent Events (SSE)，实现极低 Time-To-First-Token (TTFT)。
* **精细化流量控制**：基于 API Key 的高频访问控制，支持 5 小时、每日的 Token/Request 级别限流。

### 2. 增强型创新能力
* **智能动态路由 (Smart Routing)**：除了用户显式指定模型，支持配置 `mode="auto"`，系统根据 Prompt 的复杂度和用户的成本/延迟偏好，自动路由到性价比最高的模型（如简单任务发给 Claude 3.5 Haiku，复杂代码发给 GPT-4o）。
* **语义缓存 (Semantic Caching)**：利用向量数据库（如 Milvus/Qdrant）缓存相似 Prompt，命中缓存则直接返回，极大降低延迟和 API 成本。
* **多层级回退机制 (Fallback Engine)**：当某个上游模型提供商宕机或触发限频时，自动无缝切换到备用渠道或其他同级别模型，确保 C 端业务高可用。
* **企业级隐私合规 (Data Masking)**：提供可选的 PII（个人敏感信息）脱敏中间件，在发往公有云大模型前自动替换敏感词。

---

## 二、 整体架构设计

系统采用微服务架构，数据面追求极致性能，控制面追求高可用与强一致性。



### 1. 核心模块划分

* **API Gateway (接入层)**
    * 负责 SSL 卸载、初步的安全防护（WAF、DDoS 防护）、统一入口。
    * 推荐选型：Cloudflare (边缘加速) + APISIX / Kong。
* **Router Core (转发核心 - 数据面)**
    * 系统的绝对核心。负责请求解析、Auth 校验、限流拦截、协议转换、SSE 流式转发。
    * 推荐选型：**Golang** 或 **Rust**。Go 的高并发 Goroutine 和丰富的 AI SDK 生态非常适合，Rust 则能提供最极致的内存和吞吐表现。
* **Quota & Rate Limit Service (限流与配额服务)**
    * 负责 5h/Daily 流量控制。
    * 推荐选型：**Redis Cluster + Lua 脚本**。
* **Adapter & Proxy Engine (适配与代理引擎)**
    * 处理 `Passthrough`（原始请求透传）和 `Translation`（协议转换）。
    * 维护多个上游厂商的连接池。
* **Billing & Token Analyzer (计费与 Token 分析)**
    * 非阻塞式旁路服务。在流式输出结束时，统计真实消耗的 Input/Output Tokens 并进行扣费。

### 2. 数据库与存储设计
* **PostgreSQL**：存储账户信息、API Key 映射关系、组织架构、模型定价表。
* **Redis**：热点数据缓存（API Key 鉴权信息）、限频计数器、Session 会话状态。
* **ClickHouse**：存储海量的 API 调用日志、Token 消耗记录，用于 Dashboard 的实时多维数据分析（高吞吐写入，极速查询）。

---

## 三、 关键技术链路设计

### 1. 账户与 API Key 鉴权体系
用户在 Dashboard 生成一个以 `sk-nx-` 开头的 API Key。
请求到达 Router Core 后，系统不查询 DB，而是查询 Redis：
* **Key**: `apikey:{hash(sk-nx-...)}`
* **Value**: `{ user_id, org_id, balance, tier, status }`
这种设计保证了鉴权延迟在 1ms 以内。

### 2. 5h 与 Daily 流量控制 (Rate Limiting)
对于多维度限频（RPM - Requests Per Minute, TPD - Tokens Per Day），采用**滑动窗口 (Sliding Window)** 与**令牌桶 (Token Bucket)** 算法结合的方式。



使用 Redis Lua 脚本保证原子性。例如，计算剩余可用 Token 额度：
$Remaining = Quota_{total} - \sum_{i=1}^{N} Usage(t_i) \quad \text{where } t_i \in [T_{now} - \Delta t, T_{now}]$

* **5小时控制**：使用 Redis 的 Sorted Set，Score 为时间戳，Member 为请求 ID/Token 数，定期 `ZREMRANGEBYSCORE` 清理 5 小时前的数据。
* **Daily 控制**：简单的 Redis `INCRBY` 配合每日零点过期。

### 3. 流式传输 (Streaming) 与 Token 统计
这是系统最复杂的部分之一。标准的非流式请求很容易统计 Token，但在 SSE 流式请求中，响应是分块（Chunks）回来的。
* **转发机制**：Router Core 必须实现 HTTP Reverse Proxy 的 Flush 机制，一旦接收到上游的一个 Chunk，立刻 Flush 给客户端，绝不缓冲，保证极低延迟。
* **旁路解析**：在转发流的同时，在内存中拦截并解析这些 Chunks。
    * 如果是 OpenAI 格式，解析 `data: {"choices":[{"delta":{"content":"..."}}]}`。
    * 累计生成内容的长度，流关闭时，使用 `tiktoken` 等分词库异步计算最终 Output Token 并推送到 Kafka/RabbitMQ，由 Billing 服务进行异步扣费：
    $Cost = (Tokens_{input} \times Price_{input}) + (Tokens_{output} \times Price_{output})$

### 4. 模型适配层 (The Adapter)
* **Passthrough 模式**：如果用户请求中带有特定的 Header（例如 `X-Nexus-Target: anthropic/claude-3-opus-20240229`）且 Body 完全符合 Anthropic 规范，Router 仅替换 Authorization Header 后直接 TCP 层转发。
* **Translation 模式**：用户发送标准的 OpenAI Chat Completion 请求，模型指定为 `anthropic/claude-3`。
    * Adapter 拦截请求，提取 `messages` 数组。
    * 将 OpenAI 的 `system` role 转换为 Anthropic 的顶级 `system` 参数。
    * 将 OpenAI 的 `user`/`assistant` 转换为 Anthropic 要求的内容块。
    * 发送给 Anthropic API，接收到 Anthropic 的流式响应后，再反向包装成 OpenAI 格式的 SSE 数据流吐给客户端。

### 5. 负载均衡与会话管理 (Session Management)
为了突破单一厂商的并发限制（Rate Limits），系统内部为热门模型（如 GPT-4o）维护一个 **Key Pool（密钥池）**。
* **Round Robin / Least Connections**：轮询或最少连接数分配上游的原始 API Key。
* **会话保持**：对于需要连续对话且依赖缓存命中（如 Anthropic 的 Prompt Caching）的场景，可以通过请求头中的 `X-Session-ID`，通过一致性哈希，确保同一会话路由到相同的出口 IP 和 上游 Key。

---

## 四、 部署与扩展性规划

1.  **全球边缘节点**：将 Router Core 部署在靠近大模型服务商的数据中心（如 AWS us-east-1），以最小化与真实大模型 API 之间的网络延迟。
2.  **无状态设计**：Router Core 必须是完全无状态的，所有状态依赖 Redis 和 PostgreSQL。这样可以随时根据 QPS 使用 Kubernetes HPA（Horizontal Pod Autoscaler）进行水平扩容。
3.  **优雅降级**：当 Redis 出现抖动时，启动本地内存缓存（如 Go 语言的 `groupcache`）提供短时间内的鉴权和限流放行，保证系统的强鲁棒性。

---

这个系统设计的核心在于**高吞吐的流式代理**与**无缝的协议转换**。

