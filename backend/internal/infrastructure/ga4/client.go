// Package ga4 reads site traffic from Google Analytics — the "Сайт,
// посетители" / "Сео" columns of the leads dashboard. Read-only, same
// service account as Sheets (needs Viewer on the GA4 property + the
// Analytics Data API enabled on that service account's GCP project).
package ga4

import (
	"context"
	"fmt"
	"os"
	"time"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/analyticsdata/v1beta"
	"google.golang.org/api/option"

	"multiagent-seo/internal/domain/leadstats"
)

type Client struct {
	svc        *analyticsdata.Service
	propertyID string
}

// ga4MinStartDate is the Data API's own documented floor (2015-08-14) —
// one day past the "must be greater than 2015-08-13" it reports on a
// too-early start_date.
var ga4MinStartDate = time.Date(2015, 8, 14, 0, 0, 0, 0, time.UTC)

func New(ctx context.Context, credentialsFile, propertyID string) (*Client, error) {
	if credentialsFile == "" || propertyID == "" {
		return nil, fmt.Errorf("ga4: credentialsFile and propertyID are required")
	}
	credsJSON, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("ga4: read credentials: %w", err)
	}
	creds, err := google.CredentialsFromJSON(ctx, credsJSON, "https://www.googleapis.com/auth/analytics.readonly")
	if err != nil {
		return nil, fmt.Errorf("ga4: parse credentials: %w", err)
	}
	svc, err := analyticsdata.NewService(ctx, option.WithTokenSource(creds.TokenSource))
	if err != nil {
		return nil, fmt.Errorf("ga4: create service: %w", err)
	}
	return &Client{svc: svc, propertyID: propertyID}, nil
}

// SessionsByPeriod buckets sessions by day or month (matching leadstats'
// own Trend grouping) and splits each bucket into total vs organic-search
// sessions, in one report call — sessionDefaultChannelGroup is GA4's own
// traffic-source classification, "Organic Search" is exactly the "Сео"
// column from the old spreadsheet.
func (c *Client) SessionsByPeriod(ctx context.Context, from, to time.Time, groupBy string) ([]leadstats.TrafficBucket, error) {
	dateDim := "date"
	if groupBy == "month" {
		dateDim = "yearMonth"
	}

	// The dashboard's own "весь период" preset starts from an arbitrarily
	// early stand-in date (2000-01-01) to mean "no real lower bound" — fine
	// for Postgres, but GA4's Data API hard-rejects anything before
	// 2015-08-14 with a 400, which used to take out the *entire* traffic
	// section (mergeTraffic degrades any GA4 error to "just show zeros").
	// Clamping here means the caller never has to know GA4's floor.
	if from.Before(ga4MinStartDate) {
		from = ga4MinStartDate
	}

	resp, err := c.svc.Properties.RunReport("properties/"+c.propertyID, &analyticsdata.RunReportRequest{
		DateRanges: []*analyticsdata.DateRange{
			{StartDate: from.Format("2006-01-02"), EndDate: to.Format("2006-01-02")},
		},
		Dimensions: []*analyticsdata.Dimension{
			{Name: dateDim},
			{Name: "sessionDefaultChannelGroup"},
		},
		Metrics: []*analyticsdata.Metric{
			{Name: "sessions"},
		},
	}).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("ga4: run report: %w", err)
	}

	byBucket := map[string]*leadstats.TrafficBucket{}
	var order []string
	for _, row := range resp.Rows {
		if len(row.DimensionValues) < 2 || len(row.MetricValues) < 1 {
			continue
		}
		rawDate := row.DimensionValues[0].Value // "20240814" or "202408"
		channel := row.DimensionValues[1].Value
		var sessions int64
		fmt.Sscanf(row.MetricValues[0].Value, "%d", &sessions)

		key, err := formatBucketKey(rawDate, groupBy)
		if err != nil {
			continue
		}
		b, ok := byBucket[key]
		if !ok {
			b = &leadstats.TrafficBucket{Bucket: key}
			byBucket[key] = b
			order = append(order, key)
		}
		b.Sessions += sessions
		if channel == "Organic Search" {
			b.OrganicSessions += sessions
		}
	}

	out := make([]leadstats.TrafficBucket, 0, len(order))
	for _, key := range order {
		out = append(out, *byBucket[key])
	}
	return out, nil
}

// formatBucketKey normalizes GA4's raw date dimension ("20240814" for
// "date", "202408" for "yearMonth") to the same "2006-01-02" / "2006-01"
// strings leadstats.Bucket already uses, so buckets line up by key without
// the caller needing to know GA4's format.
func formatBucketKey(raw, groupBy string) (string, error) {
	if groupBy == "month" {
		t, err := time.Parse("200601", raw)
		if err != nil {
			return "", err
		}
		return t.Format("2006-01"), nil
	}
	t, err := time.Parse("20060102", raw)
	if err != nil {
		return "", err
	}
	return t.Format("2006-01-02"), nil
}
