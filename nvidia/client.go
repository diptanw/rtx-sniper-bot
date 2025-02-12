package nvidia

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

type (
	StockResponse struct {
		ProductTitle       string `json:"productTitle"`
		DirectPurchaseLink string `json:"directPurchaseLink"`
		PurchaseLink       string `json:"purchaseLink"`
		RetailerName       string `json:"retailerName"`
		PartnerID          string `json:"partnerId"`
		StoreID            string `json:"storeId"`
		Stock              int    `json:"stock"`
	}

	Client struct {
		client *http.Client
		apiURL *url.URL
	}

	Option func(*Client)
)

func NewClient(baseURL *url.URL, opts ...Option) *Client {
	c := Client{
		apiURL: baseURL,
	}

	for _, opt := range opts {
		opt(&c)
	}

	if c.client == nil {
		c.client = http.DefaultClient
	}

	return &c
}

func (c *Client) BuyNow(ctx context.Context, prod Product, country Country) ([]StockResponse, error) {
	params := make(url.Values)

	params.Set("sku", prod.SKU(country))
	params.Set("locale", country.Locale())

	buyNowURL := url.URL{
		Scheme:   c.apiURL.Scheme,
		Host:     c.apiURL.Host,
		Path:     "/products/v1/buy-now",
		RawQuery: params.Encode(),
	}

	req, err := http.NewRequestWithContext(ctx, "GET", buyNowURL.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("sec-ch-ua", `"Not A(Brand";v="8", "Chromium";v="132", "Google Chrome";v="132"`)
	req.Header.Set("sec-ch-ua-platform", `"macOS"`)
	req.Header.Set("sec-fetch-dest", "empty")
	req.Header.Set("sec-fetch-mode", "cors")
	req.Header.Set("sec-fetch-site", "cross-site")
	req.Header.Set("user-agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/132.0.0.0 Safari/537.36")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching stock data: %w", err)
	}

	var stockData []StockResponse

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	unescaped, err := strconv.Unquote(string(body))
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(unescaped), &stockData); err != nil {
		return nil, err
	}

	return stockData, nil
}

func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		c.client = client
	}
}
