package image

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/pkg/tlsutil"
)

// BaiduProvider 使用百度文心 ERNIE-ViLG 执行图像生成.
// 鉴权: 先 POST oauth/2.0/token 用 API Key + Secret Key 换 access_token，再 POST ernievilg/v1/txt2img；异步则轮询 getImg.
// 官方: https://ai.baidu.com/ai-doc/wenxin/
const defaultBaiduTimeout = 120 * time.Second

const defaultBaiduOAuthURL = "https://aip.baidubce.com/oauth/2.0/token"
const defaultBaiduBaseURL = "https://aip.baidubce.com"

type BaiduProvider struct {
	cfg    BaiduConfig
	client *http.Client
}

func NewBaiduProvider(cfg BaiduConfig) *BaiduProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaiduBaseURL
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultBaiduTimeout
	}
	return &BaiduProvider{
		cfg:    cfg,
		client: tlsutil.SecureHTTPClient(cfg.Timeout),
	}
}

func (p *BaiduProvider) Name() string { return "baidu" }

func (p *BaiduProvider) SupportedSizes() []string {
	return []string{"1024x1024", "720x1280", "1280x720", "768x1024", "1024x768"}
}

type baiduTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type baiduTxt2ImgRequest struct {
	Text        string `json:"text"`
	Resolution  string `json:"resolution,omitempty"`
	Style       string `json:"style,omitempty"`
	Num         int    `json:"num,omitempty"`
}

type baiduTxt2ImgResponse struct {
	TaskID int64 `json:"task_id"`
}

type baiduGetImgResponse struct {
	ImgUrls []string `json:"img_urls,omitempty"`
	Status  int      `json:"status"` // 1=处理中 2=成功 3=失败
}

func (p *BaiduProvider) getAccessToken(ctx context.Context) (string, error) {
	url := defaultBaiduOAuthURL + "?grant_type=client_credentials&client_id=" + p.cfg.APIKey + "&client_secret=" + p.cfg.SecretKey
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var t baiduTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return "", fmt.Errorf("baidu token decode: %w", err)
	}
	if t.AccessToken == "" {
		return "", fmt.Errorf("baidu token empty")
	}
	return t.AccessToken, nil
}

func (p *BaiduProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	token, err := p.getAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("baidu token: %w", err)
	}

	resolution := req.Size
	if resolution == "" {
		resolution = "1024*1024"
	} else {
		resolution = strings.ReplaceAll(resolution, "x", "*")
	}
	n := req.N
	if n <= 0 {
		n = 1
	}
	if n > 4 {
		n = 4
	}

	body := baiduTxt2ImgRequest{
		Text:       req.Prompt,
		Resolution: resolution,
		Num:        n,
	}
	payload, _ := json.Marshal(body)
	txt2imgURL := strings.TrimRight(p.cfg.BaseURL, "/") + "/rpc/2.0/ernievilg/v1/txt2img?access_token=" + token
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, txt2imgURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("baidu txt2img: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("baidu txt2img error: status=%d body=%s", resp.StatusCode, string(errBody))
	}

	var submitResp baiduTxt2ImgResponse
	if err := json.NewDecoder(resp.Body).Decode(&submitResp); err != nil {
		return nil, fmt.Errorf("baidu txt2img decode: %w", err)
	}

	taskID := submitResp.TaskID
	getImgURL := strings.TrimRight(p.cfg.BaseURL, "/") + "/rpc/2.0/ernievilg/v1/getImg?access_token=" + token
	for i := 0; i < 60; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}

		getBody, _ := json.Marshal(map[string]interface{}{"task_id": taskID})
		getReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, getImgURL, bytes.NewReader(getBody))
		getReq.Header.Set("Content-Type", "application/json")
		getResp, err := p.client.Do(getReq)
		if err != nil {
			continue
		}
		var getRespData baiduGetImgResponse
		_ = json.NewDecoder(getResp.Body).Decode(&getRespData)
		getResp.Body.Close()

		switch getRespData.Status {
		case 2:
			images := make([]ImageData, 0, len(getRespData.ImgUrls))
			for _, u := range getRespData.ImgUrls {
				if u != "" {
					images = append(images, ImageData{URL: u})
				}
			}
			return &GenerateResponse{
				Provider:  p.Name(),
				Model:     "ernie-vilg",
				Images:    images,
				Usage:     ImageUsage{ImagesGenerated: len(images)},
				CreatedAt: time.Now(),
			}, nil
		case 3:
			return nil, fmt.Errorf("baidu getImg task failed")
		}
	}

	return nil, fmt.Errorf("baidu getImg timeout")
}

func (p *BaiduProvider) Edit(ctx context.Context, req *EditRequest) (*GenerateResponse, error) {
	return nil, fmt.Errorf("baidu does not support image editing via this API")
}

func (p *BaiduProvider) CreateVariation(ctx context.Context, req *VariationRequest) (*GenerateResponse, error) {
	return nil, fmt.Errorf("baidu does not support image variations via this API")
}
