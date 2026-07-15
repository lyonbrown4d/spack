package healthcheckcmd

import (
	"context"
	"net/http"
	"time"
)

func RunForTest(url string, client *http.Client) error {
	return runHealthcheck(context.TODO(), healthcheckOptions{
		url:     url,
		timeout: time.Second,
		client:  client,
	})
}
