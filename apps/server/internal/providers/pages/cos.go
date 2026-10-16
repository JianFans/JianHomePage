package pages

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var bucketName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,51}-[0-9]{1,20}$`)
var cosRegion = regexp.MustCompile(`^[a-z]{2}-[a-z0-9]+(?:-[a-z0-9]+)*$`)
var secretID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,256}$`)

// uploadCredentials matches the ephemeral COS credentials issued by Pages.
type uploadCredentials struct {
	Bucket      string
	Region      string
	TargetPath  string
	ProjectID   *string `json:"ProjectId"`
	Credentials struct {
		ID     string `json:"TmpSecretId"`
		Secret string `json:"TmpSecretKey"`
		Token  string
	}
}

// objectKey rejects untrusted host/path material before credentials are sent to COS.
func (u uploadCredentials) objectKey(projectID string) (string, error) {
	prefix := strings.Trim(u.TargetPath, "/")
	if len(u.Bucket) > 63 || !bucketName.MatchString(u.Bucket) || !cosRegion.MatchString(u.Region) || !safePath(prefix) || !secretID.MatchString(u.Credentials.ID) || !safeHeaderValue(u.Credentials.Secret) || !safeHeaderValue(u.Credentials.Token) || (u.ProjectID != nil && *u.ProjectID != projectID) {
		return "", errors.New("invalid Pages upload credentials")
	}
	return prefix + "/artifact.zip", nil
}

// upload sends the bounded ZIP once to the SDK's fixed COS acceleration hostname.
func (c *Client) upload(ctx context.Context, u uploadCredentials, key string, archive []byte) error {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	location := url.URL{Scheme: "https", Host: u.Bucket + ".cos.accelerate.myqcloud.com", Path: "/" + key}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, location.String(), io.NopCloser(bytes.NewReader(archive)))
	if err != nil {
		return errors.New("invalid Pages upload request")
	}
	req.ContentLength = int64(len(archive))
	req.Header.Set("Content-Type", "application/zip")
	req.Header.Set("X-Cos-Security-Token", u.Credentials.Token)
	// COS uses Content-MD5 for transport integrity; it is not a security identity checksum.
	checksum := md5.Sum(archive)
	req.Header.Set("Content-MD5", base64.StdEncoding.EncodeToString(checksum[:]))
	req.Header.Set("Authorization", cosAuthorization(req, u.Credentials.ID, u.Credentials.Secret, time.Now()))
	_, err = c.send(req)
	return err
}

// cosAuthorization implements Tencent COS's documented SHA-1 HMAC request signature.
func cosAuthorization(req *http.Request, id, secret string, now time.Time) string {
	start := now.Unix() - 1
	keyTime := strconv.FormatInt(start, 10) + ";" + strconv.FormatInt(start+300, 10)
	headers := map[string]string{"host": req.URL.Host, "content-type": req.Header.Get("Content-Type"), "content-md5": req.Header.Get("Content-MD5"), "x-cos-security-token": req.Header.Get("X-Cos-Security-Token")}
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	encoded := make([]string, 0, len(keys))
	for _, key := range keys {
		encoded = append(encoded, cosEscape(key)+"="+cosEscape(headers[key]))
	}
	format := strings.ToLower(req.Method) + "\n" + req.URL.Path + "\n\n" + strings.Join(encoded, "&") + "\n"
	digest := sha1.Sum([]byte(format))
	stringToSign := "sha1\n" + keyTime + "\n" + hex.EncodeToString(digest[:]) + "\n"
	signKey := hmacSHA1(secret, keyTime)
	return "q-sign-algorithm=sha1&q-ak=" + id + "&q-sign-time=" + keyTime + "&q-key-time=" + keyTime + "&q-header-list=" + strings.Join(keys, ";") + "&q-url-param-list=&q-signature=" + hmacSHA1(signKey, stringToSign)
}

// hmacSHA1 returns the hexadecimal representation required at both COS signing stages.
func hmacSHA1(key, value string) string {
	hash := hmac.New(sha1.New, []byte(key))
	_, _ = hash.Write([]byte(value))
	return hex.EncodeToString(hash.Sum(nil))
}

// cosEscape uses RFC 3986 encoding instead of form encoding's plus sign for spaces.
func cosEscape(value string) string { return strings.ReplaceAll(url.QueryEscape(value), "+", "%20") }
