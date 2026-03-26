package replica

import (
	"net/url"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DialURLWithPrimaryCredentials injects primary credentials into a replica
// connection URL for runtime dialing while preserving the stored topology URL.
func DialURLWithPrimaryCredentials(connectionURL string, primary *pgxpool.Pool) (dialURL string) {
	dialURL = connectionURL
	defer func() {
		if recover() != nil {
			dialURL = connectionURL
		}
	}()

	if primary == nil {
		return dialURL
	}
	primaryConfig := primary.Config()
	if primaryConfig == nil || primaryConfig.ConnConfig == nil || primaryConfig.ConnConfig.User == "" {
		return dialURL
	}

	parsedURL, err := url.Parse(connectionURL)
	if err != nil {
		return dialURL
	}

	user := primaryConfig.ConnConfig.User
	password := primaryConfig.ConnConfig.Password
	if password != "" {
		parsedURL.User = url.UserPassword(user, password)
	} else {
		parsedURL.User = url.User(user)
	}
	return parsedURL.String()
}
