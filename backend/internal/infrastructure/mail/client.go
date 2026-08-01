package mail

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"multiagent-seo/internal/domain/webleads"
)

// Each call opens its own IMAP connection and closes it before returning —
// no state to track between polls.
type Client struct {
	addr     string
	username string
	password string
	folder   string
	log      *slog.Logger
}

func New(
	host string,
	port int,
	username string,
	password string,
	folder string,
	log *slog.Logger,
) *Client {
	if log == nil {
		log = slog.Default()
	}
	if folder == "" {
		folder = "INBOX"
	}
	return &Client{
		addr:     net.JoinHostPort(host, strconv.Itoa(port)),
		username: username,
		password: password,
		folder:   folder,
		log:      log,
	}
}

func (c *Client) dial() (*imapclient.Client, error) {
	imapClient, err := imapclient.DialTLS(c.addr, nil)
	if err != nil {
		return nil, fmt.Errorf("mail: dial %s: %w", c.addr, err)
	}
	if err := imapClient.Login(c.username, c.password).Wait(); err != nil {
		imapClient.Close()
		return nil, fmt.Errorf("mail: login: %w", err)
	}
	if _, err := imapClient.Select(c.folder, nil).Wait(); err != nil {
		imapClient.Close()
		return nil, fmt.Errorf("mail: select folder %q: %w", c.folder, err)
	}
	return imapClient, nil
}

// Bodies are read with PEEK, so \Seen is left untouched — callers decide
// when a message counts as processed.
func (c *Client) FetchUnseen(ctx context.Context) ([]webleads.Message, error) {
	imapClient, err := c.dial()
	if err != nil {
		return nil, err
	}
	defer imapClient.Close()

	searchData, err := imapClient.UIDSearch(&imap.SearchCriteria{
		NotFlag: []imap.Flag{imap.FlagSeen},
	}, nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("mail: search unseen: %w", err)
	}

	uids := searchData.AllUIDs()
	if len(uids) == 0 {
		return nil, nil
	}

	fetchOptions := &imap.FetchOptions{
		Envelope:    true,
		UID:         true,
		BodySection: []*imap.FetchItemBodySection{{Specifier: imap.PartSpecifierText, Peek: true}},
	}
	buffers, err := imapClient.Fetch(imap.UIDSetNum(uids...), fetchOptions).Collect()
	if err != nil {
		return nil, fmt.Errorf("mail: fetch %d message(s): %w", len(uids), err)
	}

	messages := make([]webleads.Message, 0, len(buffers))
	for _, buf := range buffers {
		messages = append(messages, toMessage(buf))
	}

	c.log.InfoContext(ctx, "mail: fetched unseen messages",
		"folder", c.folder,
		"count", len(messages),
	)
	return messages, nil
}

func (c *Client) MarkSeen(ctx context.Context, uid uint32) error {
	imapClient, err := c.dial()
	if err != nil {
		return err
	}
	defer imapClient.Close()

	storeFlags := &imap.StoreFlags{
		Op:     imap.StoreFlagsAdd,
		Silent: true,
		Flags:  []imap.Flag{imap.FlagSeen},
	}
	if err := imapClient.Store(imap.UIDSetNum(imap.UID(uid)), storeFlags, nil).Close(); err != nil {
		return fmt.Errorf("mail: mark uid %d seen: %w", uid, err)
	}

	c.log.InfoContext(ctx, "mail: marked seen", "uid", uid)
	return nil
}

func toMessage(buf *imapclient.FetchMessageBuffer) webleads.Message {
	msg := webleads.Message{UID: uint32(buf.UID)}

	if buf.Envelope != nil {
		msg.Subject = buf.Envelope.Subject
		msg.Date = buf.Envelope.Date
		msg.MessageID = buf.Envelope.MessageID
		if len(buf.Envelope.From) > 0 {
			msg.From = buf.Envelope.From[0].Addr()
		}
	}
	if len(buf.BodySection) > 0 {
		msg.Body = string(buf.BodySection[0].Bytes)
	}
	return msg
}
