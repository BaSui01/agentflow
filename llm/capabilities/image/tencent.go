package image

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/pkg/tlsutil"
)

// TencentHunyuanProvider 腾讯混元生图（腾讯云 AI 绘画 aiart）；鉴权为 TC3-HMAC-SHA256（SecretId+SecretKey）.
// 流程：SubmitTextToImageJob 提交任务 → QueryTextToImageJob 轮询直至完成，解析 ResultImage 得到图片 URL.
// 官方: https://cloud.tencent.com/document/product/1668
const defaultTencentTimeout = 120 * time.Second

const defaultTencentBaseURL = "https://aiart.tencentcloudapi.com"

const (
	tc3ActionSubmit = "SubmitTextToImageJob"
	tc3ActionQuery  = "QueryTextToImageJob"
)

type TencentHunyuanProvider struct {
	cfg    TencentHunyuanConfig
	client *http.Client
}

func NewTencentHunyuanProvider(cfg TencentHunyuanConfig) *TencentHunyuanProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultTencentBaseURL
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultTencentTimeout
	}
	return &TencentHunyuanProvider{
		cfg:    cfg,
		client: tlsutil.SecureHTTPClient(cfg.Timeout),
	}
}

func (p *TencentHunyuanProvider) Name() string { return "tencent" }

func (p *TencentHunyuanProvider) SupportedSizes() []string {
	return []string{"1024x1024", "768x1024", "1024x768", "720x1280", "1280x720", "768x768", "768x1280", "1280x768"}
}

// tencent 使用 APIKey 存 SecretId，config.SecretKey 存 SecretKey。
func (p *TencentHunyuanProvider) secretId() string  { return p.cfg.APIKey }
func (p *TencentHunyuanProvider) secretKey() string { return p.cfg.SecretKey }

func (p *TencentHunyuanProvider) doSigned(ctx context.Context, action string, body interface{}) (*http.Response, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("tencent marshal: %w", err)
	}
	baseURL := strings.TrimRight(p.cfg.BaseURL, "/")
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("tencent base url: %w", err)
	}
	host := u.Host
	if u.Port() == "" && u.Scheme == "https" {
		host = u.Hostname()
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	tc3SignRequest(httpReq, payload, p.secretId(), p.secretKey(), host, tc3ServiceAiart, tc3DefaultRegion, action, tc3AiartVersion)
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("tencent request: %w", err)
	}
	return resp, nil
}

type tencentSubmitReq struct {
	Prompt     string `json:"Prompt"`
	Resolution string `json:"Resolution,omitempty"`
	Style      string `json:"Style,omitempty"`
	LogoAdd    *int   `json:"LogoAdd,omitempty"`
	Revise     *int   `json:"Revise,omitempty"`
}

type tencentResponse struct {
	Response struct {
		JobId         string   `json:"JobId"`
		RequestId     string   `json:"RequestId"`
		JobStatusCode *int    `json:"JobStatusCode"`
		ResultImage   []string `json:"ResultImage"`
		Error         *struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		} `json:"Error,omitempty"`
	} `json:"Response"`
}

func (p *TencentHunyuanProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	resolution := req.Size
	if resolution == "" {
		resolution = "1024:1024"
	}
	resolution = strings.ReplaceAll(resolution, "x", ":")

	submitBody := tencentSubmitReq{
		Prompt:     req.Prompt,
		Resolution: resolution,
	}
	resp, err := p.doSigned(ctx, tc3ActionSubmit, submitBody)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	var tr tencentResponse
	if err := json.Unmarshal(respBody, &tr); err != nil {
		return nil, fmt.Errorf("tencent submit decode: %w", err)
	}
	if tr.Response.Error != nil {
		return nil, fmt.Errorf("tencent submit: %s %s", tr.Response.Error.Code, tr.Response.Error.Message)
	}
	jobId := tr.Response.JobId
	if jobId == "" {
		return nil, fmt.Errorf("tencent submit: empty JobId")
	}

	// 轮询 QueryTextToImageJob；JobStatusCode 1=等待 2=运行 4=失败 5=成功
	queryBody := map[string]string{"JobId": jobId}
	for i := 0; i < 60; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}

		qResp, err := p.doSigned(ctx, tc3ActionQuery, queryBody)
		if err != nil {
			continue
		}
		qBody, _ := io.ReadAll(qResp.Body)
		qResp.Body.Close()

		var qr tencentResponse
		if err := json.Unmarshal(qBody, &qr); err != nil {
			continue
		}
		if qr.Response.Error != nil {
			return nil, fmt.Errorf("tencent query: %s %s", qr.Response.Error.Code, qr.Response.Error.Message)
		}
		if qr.Response.JobStatusCode == nil {
			continue
		}
		switch *qr.Response.JobStatusCode {
		case 5:
			images := make([]ImageData, 0, len(qr.Response.ResultImage))
			for _, u := range qr.Response.ResultImage {
				if u != "" {
					images = append(images, ImageData{URL: u})
				}
			}
			return &GenerateResponse{
				Provider:  p.Name(),
				Model:     "hunyuan-aiart",
				Images:    images,
				Usage:     ImageUsage{ImagesGenerated: len(images)},
				CreatedAt: time.Now(),
			}, nil
		case 4:
			return nil, fmt.Errorf("tencent job failed (JobStatusCode=4)")
		}
	}
	return nil, fmt.Errorf("tencent query timeout")
}

func (p *TencentHunyuanProvider) Edit(ctx context.Context, req *EditRequest) (*GenerateResponse, error) {
	return nil, fmt.Errorf("tencent does not support image editing via this API")
}

func (p *TencentHunyuanProvider) CreateVariation(ctx context.Context, req *VariationRequest) (*GenerateResponse, error) {
	return nil, fmt.Errorf("tencent does not support image variations via this API")
}
