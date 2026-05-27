package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
)

type Client struct {
	token      string
	apiBase    string
	fileBase   string
	httpClient *http.Client
}

func NewClient(token string) *Client {
	return &Client{
		token:      token,
		apiBase:    "https://api.telegram.org/bot" + token,
		fileBase:   "https://api.telegram.org/file/bot" + token,
		httpClient: http.DefaultClient,
	}
}

func (c *Client) GetUpdates(ctx context.Context, offset int64, timeout int) ([]Update, error) {
	values := url.Values{}
	values.Set("offset", strconv.FormatInt(offset, 10))
	values.Set("timeout", strconv.Itoa(timeout))
	values.Set("allowed_updates", `["message"]`)

	var response struct {
		OK          bool     `json:"ok"`
		Description string   `json:"description"`
		Result      []Update `json:"result"`
	}
	if err := c.get(ctx, "/getUpdates?"+values.Encode(), &response); err != nil {
		return nil, err
	}
	if !response.OK {
		return nil, fmt.Errorf("telegram getUpdates failed: %s", response.Description)
	}
	return response.Result, nil
}

func (c *Client) SendMessage(ctx context.Context, chatID int64, text string) error {
	payload := map[string]any{
		"chat_id": chatID,
		"text":    text,
	}
	return c.postJSON(ctx, "/sendMessage", payload, nil)
}

func (c *Client) SendDocument(ctx context.Context, chatID int64, path, caption string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("chat_id", strconv.FormatInt(chatID, 10))
	if caption != "" {
		_ = writer.WriteField("caption", caption)
	}
	part, err := writer.CreateFormFile("document", filepath.Base(path))
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, file); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiBase+"/sendDocument", &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	var response struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := c.do(req, &response); err != nil {
		return err
	}
	if !response.OK {
		return fmt.Errorf("telegram sendDocument failed: %s", response.Description)
	}
	return nil
}

func (c *Client) GetFile(ctx context.Context, fileID string) (File, error) {
	values := url.Values{}
	values.Set("file_id", fileID)

	var response struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		Result      File   `json:"result"`
	}
	if err := c.get(ctx, "/getFile?"+values.Encode(), &response); err != nil {
		return File{}, err
	}
	if !response.OK {
		return File{}, fmt.Errorf("telegram getFile failed: %s", response.Description)
	}
	return response.Result, nil
}

func (c *Client) DownloadFile(ctx context.Context, filePath string, dst io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.fileBase+"/"+filePath, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("telegram file download returned status %d", resp.StatusCode)
	}

	_, err = io.Copy(dst, resp.Body)
	return err
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiBase+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

func (c *Client) postJSON(ctx context.Context, path string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiBase+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("telegram API returned status %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
