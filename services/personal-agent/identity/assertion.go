package identity

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// assertionLifetime is how long a client assertion lasts. PAP §4.1 allows at
// most 5 minutes; a minute is ample for one token request.
const assertionLifetime = 60 * time.Second

// ClientAssertion signs a private_key_jwt client assertion (PAP §4.1, RFC
// 7523 §3) for the token endpoint of the Company whose issuer is issuer. It
// is signed with the identity ES256 key, kid its thumbprint in jwks.json.
// iss and sub are the ClientID, and aud is the issuer as a single string:
// never an array and never the token endpoint, which the issuer's metadata
// could point at another Company's server (§4.1).
func (i *Identity) ClientAssertion(issuer string, now time.Time) (string, error) {
	if issuer == "" {
		return "", errors.New("identity: client assertion needs an issuer")
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"iss": i.ClientID,
		"sub": i.ClientID,
		"aud": issuer,
		"iat": now.Unix(),
		"exp": now.Add(assertionLifetime).Unix(),
		"jti": randomID(),
	})
	tok.Header["kid"] = i.es256Kid
	compact, err := tok.SignedString(i.es256)
	if err != nil {
		return "", fmt.Errorf("identity: sign client assertion: %w", err)
	}
	return compact, nil
}
