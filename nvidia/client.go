package nvidia

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

var (
	ErrNotAvailable = errors.New("product not available")
)

type (
	StockResponse struct {
		ProductTitle       string `json:"productTitle"`
		DirectPurchaseLink string `json:"directPurchaseLink"`
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

func (c *Client) BuyNow(ctx context.Context, prod Product, country Country) (map[string]string, error) {
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

	var stockData []StockResponse

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	unescaped, err := strconv.Unquote(string(body))
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(unescaped), &stockData); err != nil || len(stockData) == 0 {
		return nil, err
	}

	const (
		nvidiaID      = "111"
		nvidiaStoreID = "9595"
	)

	links := make(map[string]string)

	for _, item := range stockData {
		// Needs to be other than Nvidia partner and store.
		if item.PartnerID != nvidiaID && item.StoreID != nvidiaStoreID {
			links[item.RetailerName] = item.DirectPurchaseLink
		}
	}

	if len(links) == 0 {
		return nil, ErrNotAvailable
	}

	return links, nil
}
