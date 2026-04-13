# dnshe-ddns-go-callback

这是一个围绕 **ddns-go Callback 扩展能力** 设计的独立辅助项目，用于补足 `ddns-go -> DNSHE` 这条链路。

## 项目目的

`ddns-go` 的 Callback 机制是一个通用回调出口，每次回调提供以下变量：

- `#{ip}`：新的 IPv4 / IPv6 地址
- `#{domain}`：当前域名
- `#{recordType}`：记录类型 `A` 或 `AAAA`
- `#{ttl}`：TTL

但 DNSHE 的 `dns_records update` 接口**必填参数是 `record_id`**，而 Callback 不提供 `record_id`，导致两边接口模型不匹配：

| ddns-go Callback 提供 | DNSHE API 要求 |
|----------------------|---------------|
| IP                   | record_id（必填） |
| 域名                  | content        |
| 记录类型               | ttl            |
| TTL                  |                |

本项目提供一个桥接层，将 Callback 提供的基础信息转换成 DNSHE 所要求的"查询 + 定位 + 更新"流程。

## 关于 DNSHE 和 ddns-go

- DNSHE 官网：<https://www.dnshe.com/>
- ddns-go 官方仓库：<https://github.com/jeessy2/ddns-go>

## 许可证

本项目采用 MIT License。
