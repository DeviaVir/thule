package repo

import (
	"fmt"
	"os"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	gogitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"golang.org/x/crypto/ssh"
)

func AuthFromEnv() (transport.AuthMethod, error) {
	if token := os.Getenv("THULE_GIT_HTTP_TOKEN"); token != "" {
		user := getEnv("THULE_GIT_HTTP_USER", "oauth2")
		return &http.BasicAuth{Username: user, Password: token}, nil
	}

	if user := os.Getenv("THULE_GIT_HTTP_USER"); user != "" {
		pass := os.Getenv("THULE_GIT_HTTP_PASSWORD")
		return &http.BasicAuth{Username: user, Password: pass}, nil
	}

	keyPath := os.Getenv("THULE_GIT_SSH_KEY_PATH")
	key := os.Getenv("THULE_GIT_SSH_KEY")
	if keyPath == "" && key == "" {
		return nil, nil
	}

	user := getEnv("THULE_GIT_SSH_USER", "git")
	passphrase := os.Getenv("THULE_GIT_SSH_PASSPHRASE")

	var auth *gogitssh.PublicKeys
	var err error
	if keyPath != "" {
		auth, err = gogitssh.NewPublicKeysFromFile(user, keyPath, passphrase)
	} else {
		auth, err = gogitssh.NewPublicKeys(user, []byte(key), passphrase)
	}
	if err != nil {
		return nil, fmt.Errorf("ssh auth: %w", err)
	}

	auth.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	return auth, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
