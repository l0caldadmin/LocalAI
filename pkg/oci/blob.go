package oci

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/mudler/LocalAI/pkg/httpclient"
	"github.com/mudler/LocalAI/pkg/xio"
	digest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

func FetchImageBlob(ctx context.Context, r, reference, dst string, statusReader func(ocispec.Descriptor) io.Writer) error {
	host, repo, ok := strings.Cut(r, "/")
	if !ok || host == "" || repo == "" {
		return fmt.Errorf("failed to parse repository %q", r)
	}

	blobURL := fmt.Sprintf("https://%s/%s", host, path.Join("v2", repo, "blobs", reference))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, blobURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create blob request: %v", err)
	}
	req.Header.Set("User-Agent", UserAgent())
	req.Header.Set("Accept", "application/octet-stream")

	client := httpclient.New(httpclient.WithFollowRedirects())
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch blob: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("failed to fetch blob %s: unexpected status %s", blobURL, resp.Status)
	}

	fs, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer fs.Close()

	if statusReader != nil {
		desc := ocispec.Descriptor{Size: resp.ContentLength, Digest: digest.Digest(reference)}
		_, err = xio.Copy(ctx, io.MultiWriter(fs, statusReader(desc)), resp.Body)
	} else {
		_, err = xio.Copy(ctx, fs, resp.Body)
	}
	if err != nil {
		return err
	}

	return nil
}
