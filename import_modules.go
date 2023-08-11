package pbgo

import (
	"fmt"

	"errors"
	"time"

	// "github.com/aws/aws-lambda-go/events"
	// "github.com/aws/aws-lambda-go/lambda"
	"github.com/duke-git/lancet/v2/convertor"
	"github.com/go-resty/resty/v2"
	"golang.org/x/sync/singleflight"
)

var ErrInvalidResponse = errors.New("invalid response")

type Client struct {
	client      *resty.Client
	email       string
	password    string
	url         string
	tokenValid  time.Time
	tokenSingle singleflight.Group
}

type (
	authResponse struct {
		Token string `json:"token"`
	}

	Params struct {
		Page    int
		Size    int
		Filters string
		Sort    string
	}
)

func NewClient(url, email, password string) *Client {
	client := resty.New()
	client.
		// SetDebug(true).
		SetRetryCount(3).
		SetRetryWaitTime(3 * time.Second).
		SetRetryMaxWaitTime(10 * time.Second)

	return &Client{
		client:      client,
		url:         url,
		email:       email,
		password:    password,
		tokenSingle: singleflight.Group{},
	}
}

func (c *Client) Update(collection string, id string, body any) error {
	if err := c.auth(); err != nil {
		return err
	}

	request := c.client.R().
		SetHeader("Content-Type", "application/json").
		SetPathParam("collection", collection).
		SetBody(body)

	resp, err := request.Patch(c.url + "/api/collections/{collection}/records/" + id)
	if err != nil {
		return fmt.Errorf("[update] can't send update request to pocketbase, err %w", err)
	}
	if resp.IsError() {
		return fmt.Errorf("[update] pocketbase returned status: %d, msg: %s, err %w",
			resp.StatusCode(),
			resp.String(),
			ErrInvalidResponse,
		)
	}

	return nil
}

func (c *Client) Create(collection string, body any) ([]byte, error) {
	if err := c.auth(); err != nil {
		return nil, err
	}

	request := c.client.R().
		SetHeader("Content-Type", "application/json").
		SetPathParam("collection", collection).
		SetBody(body)

	resp, err := request.Post(c.url + "/api/collections/{collection}/records")
	if err != nil {
		//print POST request failed
		fmt.Println("POST request failed")
		return nil, err
	}

	if resp.IsError() {
		fmt.Println("Response error code: ", resp.StatusCode())
		fmt.Println("Response error string: ", resp.String())
		return nil, err
	}

	return resp.Body(), nil
}

func (c *Client) Delete(collection string, id string) error {
	if err := c.auth(); err != nil {
		return err
	}

	request := c.client.R().
		SetHeader("Content-Type", "application/json").
		SetPathParam("collection", collection).
		SetPathParam("id", id)

	resp, err := request.Delete(c.url + "/api/collections/{collection}/records/{id}")
	if err != nil {
		return fmt.Errorf("[delete] can't send update request to pocketbase, err %w", err)
	}

	if resp.IsError() {
		return fmt.Errorf("[delete] pocketbase returned status: %d, msg: %s, err %w",
			resp.StatusCode(),
			resp.String(),
			ErrInvalidResponse,
		)
	}

	return nil
}

func (c *Client) List(collection string, params Params) ([]byte, error) {
	if err := c.auth(); err != nil {
		return []byte{}, err
	}

	request := c.client.R().
		SetHeader("Content-Type", "application/json").
		SetPathParam("collection", collection)

	if params.Page > 0 {
		request.SetQueryParam("page", convertor.ToString(params.Page))
	}
	if params.Size > 0 {
		request.SetQueryParam("perPage", convertor.ToString(params.Size))
	}
	if params.Filters != "" {
		request.SetQueryParam("filter", params.Filters)
	}
	if params.Sort != "" {
		request.SetQueryParam("sort", params.Sort)
	}

	resp, err := request.Get(c.url + "/api/collections/{collection}/records")
	if err != nil {
		return []byte{}, fmt.Errorf("[list] can't send update request to pocketbase, err %w", err)
	}

	if resp.IsError() {
		return []byte{}, fmt.Errorf("[list] pocketbase returned status: %d, msg: %s, err %w",
			resp.StatusCode(),
			resp.String(),
			ErrInvalidResponse,
		)
	}

	return resp.Body(), nil
}

func (c *Client) auth() error {
	_, err, _ := c.tokenSingle.Do("auth", func() (interface{}, error) {
		if time.Now().Before(c.tokenValid) {
			return nil, nil
		}

		resp, err := c.client.R().
			SetHeader("Content-Type", "application/json").
			SetBody(map[string]interface{}{
				"identity": c.email,
				"password": c.password,
			}).
			SetResult(&authResponse{}).
			SetHeader("Authorization", "").
			Post(c.url + "/api/admins/auth-with-password")

		if err != nil {
			return nil, fmt.Errorf("[auth] can't send request to pocketbase %w", err)
		}

		if resp.IsError() {
			return nil, fmt.Errorf("[auth] pocketbase returned status: %d, msg: %s, err %w",
				resp.StatusCode(),
				resp.String(),
				ErrInvalidResponse,
			)
		}

		auth := *resp.Result().(*authResponse)
		c.client.SetHeader("Authorization", auth.Token)
		c.tokenValid = time.Now().Add(60 * time.Minute)

		return nil, nil
	})
	return err
}
