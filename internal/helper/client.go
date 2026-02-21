package helper

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/battlewithbytes/pve-appstore/internal/pct"
)

// Compile-time check that Client implements pct.HelperClient.
var _ pct.HelperClient = (*Client)(nil)

// Client connects to the helper daemon over a Unix socket.
type Client struct {
	httpClient *http.Client
	baseURL    string
	socketPath string
}

// NewClient creates a helper client that communicates over the given Unix socket.
func NewClient(socketPath string) *Client {
	return &Client{
		httpClient: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", socketPath)
				},
			},
		},
		baseURL:    "http://helper",
		socketPath: socketPath,
	}
}

// SocketPath returns the path to the Unix socket.
func (c *Client) SocketPath() string {
	return c.socketPath
}

func (c *Client) post(endpoint string, body interface{}) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}
	resp, err := c.httpClient.Post(c.baseURL+endpoint, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("helper request to %s: %w", endpoint, err)
	}
	return resp, nil
}

func decodeError(resp *http.Response) error {
	defer resp.Body.Close()
	var errResp struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		return fmt.Errorf("helper returned HTTP %d", resp.StatusCode)
	}
	return fmt.Errorf("helper: %s", errResp.Error)
}

// PctExec runs a command inside a container via the helper daemon.
func (c *Client) PctExec(ctid int, command []string) (string, int, error) {
	resp, err := c.post("/v1/pct/exec", pctExecRequest{
		CTID:    ctid,
		Command: command,
	})
	if err != nil {
		return "", -1, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", -1, decodeError(resp)
	}
	var result pctExecResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", -1, fmt.Errorf("decoding exec response: %w", err)
	}
	return result.Output, result.ExitCode, nil
}

// PctExecStream runs a command and calls onLine for each output line.
func (c *Client) PctExecStream(ctid int, command []string, onLine func(string)) (string, int, error) {
	resp, err := c.post("/v1/pct/exec-stream", pctExecRequest{
		CTID:    ctid,
		Command: command,
	})
	if err != nil {
		return "", -1, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", -1, decodeError(resp)
	}

	var output strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		output.WriteString(line)
		output.WriteByte('\n')
		if onLine != nil {
			onLine(line)
		}
	}

	exitCode := 0
	if trailer := resp.Trailer.Get("X-Exit-Code"); trailer != "" {
		exitCode, _ = strconv.Atoi(trailer)
	}

	return output.String(), exitCode, nil
}

// PctPush copies a file from the host into a container.
func (c *Client) PctPush(ctid int, src, dst, perms string) error {
	resp, err := c.post("/v1/pct/push", pctPushRequest{
		CTID:  ctid,
		Src:   src,
		Dst:   dst,
		Perms: perms,
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return decodeError(resp)
	}
	io.Copy(io.Discard, resp.Body)
	return nil
}

// PctSet modifies a container parameter via pct set.
func (c *Client) PctSet(ctid int, option, value string) error {
	resp, err := c.post("/v1/pct/set", pctSetRequest{
		CTID:   ctid,
		Option: option,
		Value:  value,
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return decodeError(resp)
	}
	io.Copy(io.Discard, resp.Body)
	return nil
}

// AppendConf appends LXC config lines to a container's config file.
func (c *Client) AppendConf(ctid int, lines []string) error {
	resp, err := c.post("/v1/conf/append", confAppendRequest{
		CTID:  ctid,
		Lines: lines,
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return decodeError(resp)
	}
	io.Copy(io.Discard, resp.Body)
	return nil
}

// Mkdir creates a directory on the host filesystem.
func (c *Client) Mkdir(path string) error {
	resp, err := c.post("/v1/fs/mkdir", fsMkdirRequest{Path: path})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return decodeError(resp)
	}
	io.Copy(io.Discard, resp.Body)
	return nil
}

// Chown changes ownership of a path on the host filesystem.
func (c *Client) Chown(path string, uid, gid int, recursive bool) error {
	resp, err := c.post("/v1/fs/chown", fsChownRequest{
		Path:      path,
		UID:       uid,
		GID:       gid,
		Recursive: recursive,
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return decodeError(resp)
	}
	io.Copy(io.Discard, resp.Body)
	return nil
}

// RemoveAll removes a path on the host filesystem.
func (c *Client) RemoveAll(path string) error {
	resp, err := c.post("/v1/fs/rm", fsRmRequest{Path: path})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return decodeError(resp)
	}
	io.Copy(io.Discard, resp.Body)
	return nil
}

// ApplyUpdate triggers the update process via the helper daemon.
func (c *Client) ApplyUpdate() error {
	resp, err := c.post("/v1/update", struct{}{})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return decodeError(resp)
	}
	io.Copy(io.Discard, resp.Body)
	return nil
}

// StartTerminal connects to the helper's terminal endpoint and returns a
// net.Conn for bidirectional PTY communication.
func (c *Client) StartTerminal(ctid int, shell string) (net.Conn, error) {
	tc, err := DialTerminal(c.socketPath, ctid, shell)
	if err != nil {
		return nil, err
	}
	return tc.conn, nil
}

// Health checks if the helper daemon is running and healthy.
func (c *Client) Health() error {
	resp, err := c.httpClient.Get(c.baseURL + "/v1/health")
	if err != nil {
		return fmt.Errorf("helper health check: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("helper returned HTTP %d", resp.StatusCode)
	}
	return nil
}
