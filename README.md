# dnshe-ddns-go-callback

> ⚠️ **项目状态：已停止维护（Archived / Unmaintained）**

本项目不再维护。  
请迁移到新的 DNSHE 专用 DDNS 项目：

👉 **DNSHE-GO：** [https://github.com/qrst1ks/dnshe-go](https://github.com/qrst1ks/dnshe-go)

---

本仓库历史代码仅保留作参考用途，不再接收功能更新或问题修复。

## 迁移建议

如果你当前仍在使用本项目，建议尽快迁移到新项目：

- 新项目地址：<https://github.com/qrst1ks/dnshe-go>
- 本项目将不再提供后续支持

# dnshe-ddns-go-callback

`ddns-go` 到 `DNSHE` 的回调桥接服务。  

## 项目开发目的

解决 `ddns-go Callback` 不提供 `record_id`、而 `DNSHE 更新接口必须要 `record_id` 的问题。

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

本项目提供一个桥接层，将 Callback 提供的基础信息转换成 DNSHE 所要求的“查询 + 定位 + 更新”流程。

## Docker（推荐）

### 方式 1：从镜像创建容器（推荐）

```bash
mkdir -p ~/docker/dnshe-ddns-go-callback/data && \
docker pull qrst1ks4/dnshe-ddns-go-callback:latest && \
docker run -d \
  --name dnshe-ddns-go-callback \
  --restart unless-stopped \
  --platform linux/arm64 \
  -p 18491:18491 \
  -e PORT=18491 \
  -v ~/docker/dnshe-ddns-go-callback/data:/data \
  qrst1ks4/dnshe-ddns-go-callback:latest
```

启动成功后访问：`http://本机IP地址:18491/`

### 方式 2：docker compose

```bash
git clone https://github.com/qrst1ks/dnshe-ddns-go-callback
cd dnshe-ddns-go-callback
docker compose up -d
```

## 本地启动

```bash
git clone https://github.com/qrst1ks/dnshe-ddns-go-callback
cd dnshe-ddns-go-callback
```

启动方式：

- macOS 双击：`start.command`
- Windows 双击：`start.bat`
- Linux/macOS 终端：`./start.sh`

默认端口：`18491`  
启动后访问：`http://本机IP地址:18491/`

## 配置步骤

### 1. 在网页填写 DNSHE API

打开首页，在「DNSHE API 配置」中填写并保存：

- `API Key`
- `API Secret`

### 2. 在 ddns-go 里配置 Callback

`URL` 填写：

`http://本机IP地址:18491/update`

`RequestBody` 填写：

```json
{"domain":"#{domain}","ip":"#{ip}","recordType":"#{recordType}","ttl":"#{ttl}"}
```

推荐：`POST` + `application/json`。

## 相关链接

- DNSHE 官网：<https://www.dnshe.com/>
- ddns-go 官方仓库：<https://github.com/jeessy2/ddns-go>

## 许可证

MIT License
