package team

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"time"
)

// LoginOptions configures the device-code enrollment loop.
type LoginOptions struct {
	APIURL     string
	OpenBrowser bool
	Stdout     func(string)
	Sleep      func(time.Duration)
}

// Login runs the device-code flow and persists credentials on success.
func Login(ctx context.Context, opt LoginOptions) (Credentials, error) {
	log := opt.Stdout
	if log == nil {
		log = func(string) {}
	}
	sleep := opt.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}

	client := NewClient(opt.APIURL, "")
	start, err := client.StartDeviceCode(ctx)
	if err != nil {
		return Credentials{}, fmt.Errorf("start device code: %w", err)
	}

	uri := start.VerificationURIComplete
	if uri == "" {
		uri = start.VerificationURI
	}
	log(fmt.Sprintf("Open %s", uri))
	log(fmt.Sprintf("User code: %s", start.UserCode))
	log("Waiting for approval in the browser…")

	if opt.OpenBrowser {
		_ = openURL(uri)
	}

	interval := time.Duration(start.Interval) * time.Second
	if interval < time.Second {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(time.Duration(start.ExpiresIn) * time.Second)
	if start.ExpiresIn <= 0 {
		deadline = time.Now().Add(15 * time.Minute)
	}

	for {
		if err := ctx.Err(); err != nil {
			return Credentials{}, err
		}
		if time.Now().After(deadline) {
			return Credentials{}, fmt.Errorf("device code expired — run kprompt login again")
		}

		poll, err := client.PollDeviceToken(ctx, start.DeviceCode)
		if err != nil {
			return Credentials{}, fmt.Errorf("poll device token: %w", err)
		}
		switch poll.Status {
		case "pending":
			sleep(interval)
			continue
		case "expired":
			return Credentials{}, fmt.Errorf("device code expired — run kprompt login again")
		case "denied":
			return Credentials{}, fmt.Errorf("device login was denied")
		case "consumed":
			return Credentials{}, fmt.Errorf("device code already used")
		case "approved":
			if poll.APIToken == "" {
				return Credentials{}, fmt.Errorf("approved but no api_token returned")
			}
			creds := Credentials{
				APIURL:    client.BaseURL,
				APIToken:  poll.APIToken,
				TokenHint: poll.TokenHint,
			}
			if poll.Org != nil {
				creds.OrgID = poll.Org.ID
				creds.OrgName = poll.Org.Name
			}
			if poll.Member != nil {
				creds.MemberEmail = poll.Member.Email
			}
			if err := SaveCredentials(creds); err != nil {
				return Credentials{}, err
			}
			return creds, nil
		default:
			return Credentials{}, fmt.Errorf("unexpected poll status %q", poll.Status)
		}
	}
}

func openURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
