package cacheddownloader

import (
	"bytes"
	"crypto/md5"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const MAX_DOWNLOAD_ATTEMPTS = 3

type Downloader struct {
	client                    *http.Client
	concurrentDownloadBarrier chan struct{}
}

func NewDownloader(timeout time.Duration, maxConcurrentDownloads int, skipSSLVerification bool) *Downloader {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		Dial: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: skipSSLVerification,
			MinVersion:         tls.VersionTLS10,
		},
		ResponseHeaderTimeout: timeout,
	}
	client := &http.Client{
		Transport: transport,
	}

	return &Downloader{
		client: client,
		concurrentDownloadBarrier: make(chan struct{}, maxConcurrentDownloads),
	}
}

func (downloader *Downloader) Download(url *url.URL, createDestination func() (*os.File, error), cachingInfoIn CachingInfoType) (path string, cachingInfoOut CachingInfoType, err error) {
	downloader.concurrentDownloadBarrier <- struct{}{}
	defer func() {
		<-downloader.concurrentDownloadBarrier
	}()

	for attempt := 0; attempt < MAX_DOWNLOAD_ATTEMPTS; attempt++ {
		path, cachingInfoOut, err = downloader.fetchToFile(url, createDestination, cachingInfoIn)

		if err == nil {
			break
		}
	}

	if err != nil {
		return "", CachingInfoType{}, err
	}

	return
}

func (downloader *Downloader) fetchToFile(url *url.URL, createDestination func() (*os.File, error), cachingInfoIn CachingInfoType) (string, CachingInfoType, error) {
	var req *http.Request
	var err error

	req, err = http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return "", CachingInfoType{}, err
	}

	if cachingInfoIn.ETag != "" {
		req.Header.Add("If-None-Match", cachingInfoIn.ETag)
	}
	if cachingInfoIn.LastModified != "" {
		req.Header.Add("If-Modified-Since", cachingInfoIn.LastModified)
	}

	var resp *http.Response
	resp, err = downloader.client.Do(req)
	if err != nil {
		return "", CachingInfoType{}, err
	}

	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return "", CachingInfoType{}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return "", CachingInfoType{}, fmt.Errorf("Download failed: Status code %d", resp.StatusCode)
	}

	var destinationFile *os.File
	destinationFile, err = createDestination()
	if err != nil {
		return "", CachingInfoType{}, err
	}

	defer func() {
		destinationFile.Close()
		if err != nil {
			os.Remove(destinationFile.Name())
		}
	}()

	_, err = destinationFile.Seek(0, 0)
	if err != nil {
		return "", CachingInfoType{}, err
	}

	err = destinationFile.Truncate(0)
	if err != nil {
		return "", CachingInfoType{}, err
	}

	hash := md5.New()

	_, err = io.Copy(io.MultiWriter(destinationFile, hash), resp.Body)
	if err != nil {
		return "", CachingInfoType{}, err
	}

	cachingInfoOut := CachingInfoType{
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
	}

	etagChecksum, ok := convertETagToChecksum(cachingInfoOut.ETag)

	if ok && !bytes.Equal(etagChecksum, hash.Sum(nil)) {
		err = fmt.Errorf("Download failed: Checksum mismatch")
		return "", CachingInfoType{}, err
	}

	return destinationFile.Name(), cachingInfoOut, nil
}

// convertETagToChecksum returns true if ETag is a valid MD5 hash, so a checksum action was intended.
// See here for our motivation: http://docs.aws.amazon.com/AmazonS3/latest/API/RESTCommonResponseHeaders.html
func convertETagToChecksum(etag string) ([]byte, bool) {
	etag = strings.Trim(etag, `"`)

	if len(etag) != 32 {
		return nil, false
	}

	c, err := hex.DecodeString(etag)
	if err != nil {
		return nil, false
	}

	return c, true
}
