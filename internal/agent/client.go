package agent

import (
	"fmt"
	"net"
	"time"
)

const dialTimeout = 2 * time.Second

// Get retrieves a value from the agent.
func Get(sockPath, key string) (string, error) {
	resp, err := roundTrip(sockPath, &Request{Payload: &Request_Get{Get: &GetRequest{Key: key}}})
	if err != nil {
		return "", err
	}
	if !resp.Ok {
		return "", fmt.Errorf("%s", resp.Error)
	}
	return resp.Value, nil
}

// List retrieves all keys from the agent.
func List(sockPath string) ([]string, error) {
	resp, err := roundTrip(sockPath, &Request{Payload: &Request_List{List: &ListRequest{}}})
	if err != nil {
		return nil, err
	}
	if !resp.Ok {
		return nil, fmt.Errorf("%s", resp.Error)
	}
	return resp.Keys, nil
}

// Set stores a key-value pair via the agent.
// Passphrase is required when overwriting an existing key.
func Set(sockPath, key, value, passphrase string) error {
	resp, err := roundTrip(sockPath, &Request{Payload: &Request_Set{Set: &SetRequest{Key: key, Value: value, Passphrase: passphrase}}})
	if err != nil {
		return err
	}
	if !resp.Ok {
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}

// Delete removes a key via the agent. Passphrase is always required.
func Delete(sockPath, key, passphrase string) error {
	resp, err := roundTrip(sockPath, &Request{Payload: &Request_Delete{Delete: &DeleteRequest{Key: key, Passphrase: passphrase}}})
	if err != nil {
		return err
	}
	if !resp.Ok {
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}

// Passwd changes the store passphrase via the agent.
func Passwd(sockPath, oldPass, newPass string) error {
	resp, err := roundTrip(sockPath, &Request{Payload: &Request_Passwd{Passwd: &PasswdRequest{Passphrase: oldPass, NewPassphrase: newPass}}})
	if err != nil {
		return err
	}
	if !resp.Ok {
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}

// Stop signals the agent to wipe memory and exit.
func Stop(sockPath string) error {
	resp, err := roundTrip(sockPath, &Request{Payload: &Request_Stop{Stop: &StopRequest{}}})
	if err != nil {
		return err
	}
	if !resp.Ok {
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}

// IsRunning checks if an agent is reachable at the given socket path.
func IsRunning(sockPath string) bool {
	conn, err := net.DialTimeout("unix", sockPath, dialTimeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func roundTrip(sockPath string, req *Request) (*Response, error) {
	conn, err := net.DialTimeout("unix", sockPath, dialTimeout)
	if err != nil {
		return nil, fmt.Errorf("connecting to agent: %w", err)
	}
	defer conn.Close()

	// Write operations (set, delete, passwd) do scrypt encryption which
	// can take ~500ms. Use a generous deadline.
	conn.SetDeadline(time.Now().Add(30 * time.Second))

	if err := WriteMsg(conn, req); err != nil {
		return nil, fmt.Errorf("writing to agent: %w", err)
	}

	var resp Response
	if err := ReadMsg(conn, &resp); err != nil {
		return nil, fmt.Errorf("reading from agent: %w", err)
	}
	return &resp, nil
}
