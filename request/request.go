package request

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"time"
)

func Post(url string, data interface{}) string {
	e, err := json.Marshal(data)
	if err != nil {
		return ""
	}

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer([]byte(string(e))))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	return string(body)
}

func Get(url string) string {
	transCfg := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // ignore expired SSL certificates
	}
	client := &http.Client{Transport: transCfg}

	response, err := client.Get(url)
	if err != nil {
		return "error"
	}

	if response == nil {
		return "error"
	}

	defer response.Body.Close()

	htmlData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "error"
	}

	return string(htmlData)
}
