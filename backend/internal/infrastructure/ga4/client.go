// Package ga4 reads site traffic from Google Analytics — the "Сайт,
// посетители" / "Сео" columns of the leads dashboard. Read-only, same
// service account as Sheets (needs Viewer on the GA4 property + the
// Analytics Data API enabled on that service account's GCP project).
package ga4

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/analyticsdata/v1beta"
	"google.golang.org/api/option"

	"multiagent-seo/internal/domain/finance"
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
		// A metric that doesn't parse must not silently read as zero traffic —
		// that understates the dashboard's sessions with no signal at all.
		sessions, err := strconv.ParseInt(row.MetricValues[0].Value, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("ga4: parse sessions %q for %q: %w", row.MetricValues[0].Value, rawDate, err)
		}

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

// Audience reports visitor demographics/geography for the period —
// userAgeBracket and userGender are GA4's modeled estimates (not
// self-reported), city is coarse IP geolocation. All three dimensions come
// back in one report as combinatorial rows (age × gender × city); rather
// than make the caller unpack that, each is summed independently here, so
// the result reads as three separate breakdowns.
func (c *Client) Audience(ctx context.Context, from, to time.Time) (leadstats.Audience, error) {
	if from.Before(ga4MinStartDate) {
		from = ga4MinStartDate
	}

	resp, err := c.svc.Properties.RunReport("properties/"+c.propertyID, &analyticsdata.RunReportRequest{
		DateRanges: []*analyticsdata.DateRange{
			{StartDate: from.Format("2006-01-02"), EndDate: to.Format("2006-01-02")},
		},
		Dimensions: []*analyticsdata.Dimension{
			{Name: "userAgeBracket"},
			{Name: "userGender"},
			{Name: "city"},
		},
		Metrics: []*analyticsdata.Metric{
			{Name: "sessions"},
		},
	}).Context(ctx).Do()
	if err != nil {
		return leadstats.Audience{}, fmt.Errorf("ga4: audience report: %w", err)
	}

	byAge := map[string]int64{}
	byGender := map[string]int64{}
	byCity := map[string]int64{}
	for _, row := range resp.Rows {
		if len(row.DimensionValues) < 3 || len(row.MetricValues) < 1 {
			continue
		}
		sessions, err := strconv.ParseInt(row.MetricValues[0].Value, 10, 64)
		if err != nil {
			return leadstats.Audience{}, fmt.Errorf("ga4: parse audience sessions %q: %w", row.MetricValues[0].Value, err)
		}
		byAge[row.DimensionValues[0].Value] += sessions
		byGender[row.DimensionValues[1].Value] += sessions
		byCity[row.DimensionValues[2].Value] += sessions
	}

	return leadstats.Audience{
		ByAge:    countsDesc(byAge, 0),
		ByGender: countsDesc(byGender, 0),
		// The site draws visitors from far more cities than are useful to
		// show at once — cap to the top 10 by sessions, long tail dropped.
		ByCity: countsDesc(byCity, 10),
	}, nil
}

// countsDesc turns a key->sessions map into Counts sorted by count
// descending (ties broken by key, for a stable order across calls), capped
// to the first limit rows — 0 means no cap.
func countsDesc(counts map[string]int64, limit int) []leadstats.Count {
	out := make([]leadstats.Count, 0, len(counts))
	for k, v := range counts {
		out = append(out, leadstats.Count{Key: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// AdSpend reports what GA4 calls Google Ads cost per month, read on the same
// service account that reads sessions — no Ads API credentials involved.
//
// DIAGNOSTIC ONLY. On this property the figure is NOT a usable money value: it
// is non-additive across query windows (December alone reads 323 626, December
// inside a two-month range reads 796 004), and no single scale factor maps it
// onto payments the company actually made (ratios of 12.5 / 19.2 / 23.5 for
// December / January / February 2025). GA4 also refuses advertiserAdCost
// without an ads-scoped dimension, which is what drives the re-attribution.
// Real ad-spend automation needs the Google Ads API (developer token + OAuth +
// customer ID), which nobody has set up yet — see cmd/adspend.
func (c *Client) AdSpend(ctx context.Context, from, to time.Time) ([]finance.AdSpend, error) {
	resp, err := c.svc.Properties.RunReport("properties/"+c.propertyID, &analyticsdata.RunReportRequest{
		DateRanges: []*analyticsdata.DateRange{{
			StartDate: from.Format("2006-01-02"),
			EndDate:   to.Format("2006-01-02"),
		}},
		Dimensions: []*analyticsdata.Dimension{{Name: "yearMonth"}, {Name: "sessionCampaignName"}},
		Metrics: []*analyticsdata.Metric{
			{Name: "advertiserAdCost"},
			{Name: "advertiserAdClicks"},
		},
	}).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("ga4: ad spend report: %w", err)
	}

	byMonth := map[string]*finance.AdSpend{}
	var order []string
	for _, row := range resp.Rows {
		if len(row.DimensionValues) < 1 || len(row.MetricValues) < 2 {
			continue
		}
		// Cost GA4 could not attribute to a session month comes back with an
		// empty yearMonth. It is real money, so it is reported under an empty
		// Month rather than dropped — a caller that needs monthly buckets can
		// see how much never made it into one.
		raw := row.DimensionValues[0].Value // "202412", or "" when unattributed
		month := ""
		if len(raw) == 6 {
			month = raw[:4] + "-" + raw[4:]
		} else if raw != "" {
			return nil, fmt.Errorf("ga4: unexpected yearMonth %q", raw)
		}

		cost, err := strconv.ParseFloat(row.MetricValues[0].Value, 64)
		if err != nil {
			return nil, fmt.Errorf("ga4: parse ad cost %q for %s: %w", row.MetricValues[0].Value, month, err)
		}
		clicks, err := strconv.ParseInt(row.MetricValues[1].Value, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("ga4: parse ad clicks %q for %s: %w", row.MetricValues[1].Value, month, err)
		}

		m, ok := byMonth[month]
		if !ok {
			m = &finance.AdSpend{Month: month}
			byMonth[month] = m
			order = append(order, month)
		}
		m.Cost += cost
		m.Clicks += clicks
	}

	sort.Strings(order)
	out := make([]finance.AdSpend, 0, len(order))
	for _, month := range order {
		out = append(out, *byMonth[month])
	}
	return out, nil
}
