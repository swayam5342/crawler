package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"time"

	"github.com/swayamsb50/web_crawler/internal/extractor"
	"github.com/swayamsb50/web_crawler/internal/fetcher"
	"github.com/swayamsb50/web_crawler/internal/storage"
	"github.com/swayamsb50/web_crawler/internal/storage/blobstore"
	"github.com/swayamsb50/web_crawler/internal/urlutil"
)

// pageFetcher adapts *fetcher.Client plus an extractor.Extractor into
// crawler.PageFetcher: it fetches a page, extracts links and metadata,
// persists the page (metadata to storage.Backend, raw body to
// blobstore.Store, both optional), and returns the links worth following.
type pageFetcher struct {
	client    *fetcher.Client
	extractor extractor.Extractor
	storage   storage.Backend // optional
	blobs     blobstore.Store // optional

	honorNofollow bool
	honorNoindex  bool

	logger *slog.Logger
}

func (p *pageFetcher) Fetch(ctx context.Context, rawURL string) ([]string, error) {
	res, err := p.client.Fetch(ctx, rawURL)
	if err != nil {
		return nil, err
	}

	extracted, err := p.extractor.Extract(res.FinalURL, res.Body)
	if err != nil {
		p.logger.Warn("extraction failed; page will not be stored and no links followed", "url", rawURL, "error", err)
		return nil, nil
	}

	p.persist(ctx, rawURL, res, extracted)

	links := make([]string, 0, len(extracted.Links))
	for _, l := range extracted.Links {
		if p.honorNofollow && l.NoFollow {
			continue
		}
		links = append(links, l.URL)
	}
	return links, nil
}

func (p *pageFetcher) persist(ctx context.Context, rawURL string, res *fetcher.Result, extracted extractor.Result) {
	if p.storage == nil && p.blobs == nil {
		return
	}
	if p.honorNoindex && extracted.NoIndex {
		return
	}

	domain, err := urlutil.Domain(res.FinalURL)
	if err != nil {
		p.logger.Warn("could not derive domain for storage", "url", rawURL, "error", err)
		return
	}
	hash := contentHash(res.Body)

	if p.storage != nil {
		page := storage.Page{
			URL:         res.FinalURL,
			Domain:      domain,
			StatusCode:  res.StatusCode,
			Title:       extracted.Title,
			Language:    extracted.Language,
			NoIndex:     extracted.NoIndex,
			ContentHash: hash,
			FetchedAt:   time.Now().UTC(),
		}
		if err := p.storage.SavePage(ctx, page); err != nil {
			p.logger.Warn("failed to save page metadata", "url", rawURL, "error", err)
		}
	}

	if p.blobs != nil {
		if err := p.blobs.Put(ctx, hash, res.Body); err != nil {
			p.logger.Warn("failed to save page blob", "url", rawURL, "error", err)
		}
	}
}

func contentHash(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
