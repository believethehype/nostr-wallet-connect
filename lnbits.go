package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/tidwall/sjson"
)

type LNBitsOptions struct {
	AdminKey string
	Host     string
}

type LNBitsWrapper struct {
	client  *LNClient
	options LNBitsOptions
}

func (lnbits *LNBitsWrapper) GetBalance(ctx context.Context, senderPubkey string) (balance int64, err error) {
	httpclient := &http.Client{
		Timeout: 10 * time.Second,
	}
	req, err := http.NewRequest("GET",
		lnbits.options.Host+"/api/v1/wallet",
		nil,
	)
	if err != nil {
		return 0, err
	}

	req.Header.Set("X-Api-Key", lnbits.options.AdminKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpclient.Do(req)
	if err != nil {
		return
	}

	if resp.StatusCode >= 300 {
		body, _ := ioutil.ReadAll(resp.Body)
		text := string(body)
		if len(text) > 300 {
			text = text[:300]
		}
		return 0, fmt.Errorf("call to lnbits failed (%d): %s", resp.StatusCode, text)
	}

	defer resp.Body.Close()
	responseData, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Print(err.Error())
		return
	}
	var jsonMap map[string]interface{}
	err = json.Unmarshal([]byte(string(responseData)), &jsonMap)
	if err != nil {
		fmt.Print(err.Error())
		return
	}
	return jsonMap["balance"].(int64), nil
}

func (lnbits *LNBitsWrapper) SendPaymentSync(ctx context.Context, senderPubkey, payReq string) (preimage string, err error) {
	httpclient := &http.Client{
		Timeout: 10 * time.Second,
	}
	body, _ := sjson.Set("{}", "out", true)
	body, _ = sjson.Set(body, "bolt11", payReq)

	req, err := http.NewRequest("POST",
		lnbits.options.Host+"/api/v1/payments",
		bytes.NewBufferString(body),
	)
	if err != nil {
		return "", err
	}

	req.Header.Set("X-Api-Key", lnbits.options.AdminKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpclient.Do(req)
	if err != nil {
		return
	}

	if resp.StatusCode >= 300 {
		body, _ := ioutil.ReadAll(resp.Body)
		text := string(body)
		if len(text) > 300 {
			text = text[:300]
		}
		return "", fmt.Errorf("call to lnbits failed (%d): %s", resp.StatusCode, text)
	}

	defer resp.Body.Close()
	responseData, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Print(err.Error())
		return
	}
	var jsonMap map[string]interface{}
	err = json.Unmarshal([]byte(string(responseData)), &jsonMap)
	if err != nil {
		fmt.Print(err.Error())
		return
	}
	return jsonMap["payment_hash"].(string), nil
}
