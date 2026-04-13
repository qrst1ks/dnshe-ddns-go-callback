package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var dnsheBase = "https://api005.dnshe.com/index.php?m=domain_hub"

var client = &http.Client{Timeout: 20 * time.Second}

// UpdateRequest ddns-go callback 请求体
type UpdateRequest struct {
	Domain     string `json:"domain"`
	IP         string `json:"ip"`
	RecordType string `json:"recordType"`
	TTL        int    `json:"ttl"`
}

// dnsheHeaders 返回请求 DNSHE API 所需的 header
func dnsheHeaders() http.Header {
	key, secret := getRuntimeConfig()
	h := make(http.Header)
	h.Set("X-API-Key", key)
	h.Set("X-API-Secret", secret)
	h.Set("Content-Type", "application/json")
	return h
}

// FindSubdomain 查找子域名 ID
func FindSubdomain(domain string) (int, error) {
	url := dnsheBase + "&endpoint=subdomains&action=list"

	body, err := dnsheGet(url)
	if err != nil {
		return 0, err
	}
	if err := validateDNSHEResult(body); err != nil {
		return 0, err
	}

	var resp struct {
		Subdomains []struct {
			ID         int    `json:"id"`
			FullDomain string `json:"full_domain"`
		} `json:"subdomains"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, fmt.Errorf("parse subdomains response: %w", err)
	}

	for _, s := range resp.Subdomains {
		if s.FullDomain == domain {
			return s.ID, nil
		}
	}
	return 0, nil
}

// FindRecord 查找 DNS 记录，返回 record_id 和当前 IP
func FindRecord(subdomainID int, recordType string) (int, string, error) {
	url := fmt.Sprintf("%s&endpoint=dns_records&action=list&subdomain_id=%d", dnsheBase, subdomainID)

	body, err := dnsheGet(url)
	if err != nil {
		return 0, "", err
	}
	if err := validateDNSHEResult(body); err != nil {
		return 0, "", err
	}

	var resp struct {
		Records []struct {
			ID      int    `json:"id"`
			Type    string `json:"type"`
			Content string `json:"content"`
		} `json:"records"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, "", fmt.Errorf("parse records response: %w", err)
	}

	for _, r := range resp.Records {
		if r.Type == recordType {
			return r.ID, r.Content, nil
		}
	}
	return 0, "", nil
}

// UpdateRecord 更新 DNS 记录
func UpdateRecord(recordID int, ip string, ttl int) (map[string]any, error) {
	url := dnsheBase + "&endpoint=dns_records&action=update"

	payload := map[string]any{
		"record_id": recordID,
		"content":   ip,
		"ttl":       ttl,
	}

	body, err := dnshePost(url, payload)
	if err != nil {
		return nil, err
	}
	if err := validateDNSHEResult(body); err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse update response: %w", err)
	}
	return result, nil
}

// dnsheGet 发送 GET 请求到 DNSHE
func dnsheGet(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header = dnsheHeaders()

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("DNSHE API error %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// dnshePost 发送 POST 请求到 DNSHE
func dnshePost(url string, payload any) ([]byte, error) {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header = dnsheHeaders()

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("DNSHE API error %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// validateDNSHEResult 检查 DNSHE 的业务层返回状态（不仅是 HTTP 状态码）。
func validateDNSHEResult(body []byte) error {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		// 某些 list 接口可能返回非对象，交由后续具体解析处理
		return nil
	}

	success := strings.TrimSpace(strings.ToLower(stringFromAny(m["success"])))
	if success == "false" || success == "0" || success == "no" {
		return fmt.Errorf("DNSHE API business error: %s", firstNonEmpty(stringFromAny(m["msg"]), stringFromAny(m["message"]), "unknown error"))
	}

	status := strings.TrimSpace(strings.ToLower(stringFromAny(m["status"])))
	if status == "error" || status == "failed" || status == "fail" {
		return fmt.Errorf("DNSHE API business error: %s", firstNonEmpty(stringFromAny(m["msg"]), stringFromAny(m["message"]), "unknown error"))
	}

	codeStr := strings.TrimSpace(stringFromAny(m["code"]))
	if codeStr != "" {
		if code, err := strconv.Atoi(codeStr); err == nil && code != 0 {
			return fmt.Errorf("DNSHE API code=%d: %s", code, firstNonEmpty(stringFromAny(m["msg"]), stringFromAny(m["message"]), "unknown error"))
		}
	}

	if errMsg := firstNonEmpty(stringFromAny(m["error"]), stringFromAny(m["err"])); errMsg != "" {
		return fmt.Errorf("DNSHE API business error: %s", errMsg)
	}

	return nil
}

// maskIP 日志中脱敏 IP
func maskIP(ip string) string {
	if ip == "" {
		return "-"
	}
	if strings.Contains(ip, ":") {
		parts := strings.Split(ip, ":")
		if len(parts) > 4 {
			return strings.Join(parts[:2], ":") + ":****:" + parts[len(parts)-1]
		}
		return ip
	}
	if strings.Contains(ip, ".") {
		parts := strings.Split(ip, ".")
		if len(parts) == 4 {
			return parts[0] + "." + parts[1] + ".***.***"
		}
	}
	return ip
}
