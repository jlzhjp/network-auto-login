package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const ConnectivityCheckURL = "http://connect.rom.miui.com/generate_204"
const AuthenticationURL = "http://128.192.100.4:8080/cgi-bin/ace_web_auth.cgi"

func loadEnvVars() (username, password string, fwmark int, err error) {
	username, ok := os.LookupEnv("NETWORK_LOGIN_USERNAME")
	if !ok {
		return "", "", 0, fmt.Errorf("NETWORK_LOGIN_USERNAME environment variable not set")
	}

	password, ok = os.LookupEnv("NETWORK_LOGIN_PASSWORD")
	if !ok {
		return "", "", 0, fmt.Errorf("NETWORK_LOGIN_PASSWORD environment variable not set")
	}

	fwmarkStr, ok := os.LookupEnv("NETWORK_LOGIN_FWMARK")
	if !ok {
		return username, password, 0, nil
	}

	fwmark64, err := strconv.ParseInt(fwmarkStr, 0, 32)
	if err != nil {
		return "", "", 0, fmt.Errorf("invalid NETWORK_LOGIN_FWMARK value: %v", err)
	}
	return username, password, int(fwmark64), nil
}

func createHttpClient(fwmark int) *http.Client {
	dialer := &net.Dialer{}

	if fwmark != 0 {
		dialer = &net.Dialer{
			Control: func(network string, address string, c syscall.RawConn) error {
				var sockErr error
				err := c.Control(func(fd uintptr) {
					sockErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_MARK, fwmark)
				})
				if err != nil {
					return err
				}
				return sockErr
			},
		}
	}

	dialer.Resolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, "223.5.5.5:53")
		},
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = dialer.DialContext

	client := &http.Client{
		Transport: transport,
	}

	return client
}

func authenticate(client *http.Client, username, password string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	formData := url.Values{
		"username": {username},
		"userpwd":  {password},
	}
	body := strings.NewReader(formData.Encode())

	req, err := http.NewRequestWithContext(ctx, "POST", AuthenticationURL, body)

	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	// TODO: check the content of the response body to verify successful authentication
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("authentication failed with status code: %d", resp.StatusCode)
	}

	return nil
}

func checkConnectivity(client *http.Client) (statusCode int, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", ConnectivityCheckURL, nil)

	if err != nil {
		return -1, err
	}

	resp, err := client.Do(req)

	if err != nil {
		return -1, err
	}

	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	return resp.StatusCode, nil
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	username, password, fwmark, err := loadEnvVars()
	if err != nil {
		slog.Error("error loading environment variables", "error", err)
		return
	}

	client := createHttpClient(fwmark)
	statusCode, err := checkConnectivity(client)

	if err != nil {
		slog.Error("connectivity check failed", "error", err)
		return
	}

	switch statusCode {
	case 200:
		err = authenticate(client, username, password)
		if err != nil {
			slog.Error("authentication failed", "error", err)
			return
		}
		slog.Info("authentication successful")
	case 204:
		slog.Info("already connected to the internet")
	default:
		slog.Warn("unexpected response status", "status", statusCode)
	}
}
