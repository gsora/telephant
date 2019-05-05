// Package mastodon is a Mastodon account for Chirp.
package mastodon

import (
	"context"
	"fmt"
	"log"
	"regexp"

	"github.com/davecgh/go-spew/spew"
	"github.com/mattn/go-mastodon"

	"github.com/muesli/chirp/accounts"
)

const (
	initialFeedCount          = 40
	initialNotificationsCount = 40
)

// Account is a Mastodon account for Chirp.
type Account struct {
	client *mastodon.Client
	config *mastodon.Config
	self   *mastodon.Account

	evchan  chan interface{}
	SigChan chan bool
}

// NewAccount returns a new Mastodon account.
func NewAccount(instance, token, clientID, clientSecret string) *Account {
	mconfig := &mastodon.Config{
		Server:       instance,
		AccessToken:  token,
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}

	return &Account{
		config: mconfig,
		client: mastodon.NewClient(mconfig),
	}
}

func RegisterAccount(instance string) (*Account, string, string, error) {
	app, err := mastodon.RegisterApp(context.Background(), &mastodon.AppConfig{
		Server:     instance,
		ClientName: "Telephant",
		Scopes:     "read write follow post",
		Website:    "",
	})
	if err != nil {
		return nil, "", "", err
	}

	a := NewAccount(instance, "", app.ClientID, app.ClientSecret)

	return a, app.AuthURI, app.RedirectURI, nil
}

func (mod *Account) Authenticate(code string) (string, string, string, string, error) {
	err := mod.client.AuthenticateToken(context.Background(), code, "urn:ietf:wg:oauth:2.0:oob")
	if err != nil {
		return "", "", "", "", err
	}

	return mod.config.Server, mod.config.AccessToken, mod.config.ClientID, mod.config.ClientSecret, nil
}

// Run executes the account's event loop.
func (mod *Account) Run(eventChan chan interface{}) {
	mod.evchan = eventChan

	if mod.config.AccessToken == "" {
		return
	}

	var err error
	mod.self, err = mod.client.GetAccountCurrentUser(context.Background())
	if err != nil {
		panic(err)
	}

	ev := accounts.LoginEvent{
		Username:   mod.self.Username,
		Name:       mod.self.DisplayName,
		Avatar:     mod.self.Avatar,
		ProfileURL: mod.self.URL,
		Posts:      mod.self.StatusesCount,
		Follows:    mod.self.FollowingCount,
		Followers:  mod.self.FollowersCount,
	}
	mod.evchan <- ev

	// seed feeds initially
	nn, err := mod.client.GetNotifications(context.Background(), &mastodon.Pagination{
		Limit: initialNotificationsCount,
	})
	if err != nil {
		panic(err)
	}
	for _, n := range nn {
		mod.handleNotification(n)
	}

	tt, err := mod.client.GetTimelineHome(context.Background(), &mastodon.Pagination{
		Limit: initialFeedCount,
	})
	if err != nil {
		panic(err)
	}
	for _, t := range tt {
		mod.handleStatus(t)
	}

	mod.handleStream()
}

func (mod *Account) Logo() string {
	return "mastodon.svg"
}

// Post posts a new status
func (mod *Account) Post(message string) error {
	_, err := mod.client.PostStatus(context.Background(), &mastodon.Toot{
		Status: message,
	})
	return err
}

// Reply posts a new reply-status
func (mod *Account) Reply(replyid string, message string) error {
	_, err := mod.client.PostStatus(context.Background(), &mastodon.Toot{
		Status:      message,
		InReplyToID: mastodon.ID(replyid),
	})
	return err
}

// Share boosts a post
func (mod *Account) Share(id string) error {
	_, err := mod.client.Reblog(context.Background(), mastodon.ID(id))
	return err
}

// Like favourites a post
func (mod *Account) Like(id string) error {
	_, err := mod.client.Favourite(context.Background(), mastodon.ID(id))
	return err
}

func handleRetweetStatus(status string) string {
	/*
		if strings.HasPrefix(status, "RT ") && strings.Count(status, " ") >= 2 {
			return strings.Join(strings.Split(status, " ")[2:], " ")
		}
	*/

	return status
}

func handleReplyStatus(status string) string {
	/*
		if strings.HasPrefix(status, "@") && strings.Index(status, " ") > 0 {
			return status[strings.Index(status, " "):]
		}
	*/

	return status
}

func parseBody(body string) string {
	r := regexp.MustCompile("<span class=\"invisible\">(.[^<]*)</span>")
	body = r.ReplaceAllString(body, "")

	r = regexp.MustCompile("<span class=\"ellipsis\">(.[^<]*)</span>")
	return r.ReplaceAllString(body, "$1...")

	/*
		for _, u := range ents.Urls {
			r := fmt.Sprintf("<a style=\"text-decoration: none; color: orange;\" href=\"%s\">%s</a>", u.Expanded_url, u.Display_url)
			ev.Post.Body = strings.Replace(ev.Post.Body, u.Url, r, -1)
		}
		for _, media := range ents.Media {
			ev.Media = append(ev.Media, media.Media_url_https)
			ev.Post.Body = strings.Replace(ev.Post.Body, media.Url, "", -1)
			// FIXME:
			break
		}
	*/
}

func (mod *Account) handleNotification(n *mastodon.Notification) {
	var ev accounts.MessageEvent
	if n.Status != nil {
		ev = accounts.MessageEvent{
			Account:      "mastodon",
			Name:         "post",
			Notification: true,

			Post: accounts.Post{
				MessageID:  string(n.Status.ID),
				Body:       parseBody(n.Status.Content),
				Author:     n.Account.Username,
				AuthorName: n.Account.DisplayName,
				AuthorURL:  n.Account.URL,
				Avatar:     n.Account.Avatar,
				CreatedAt:  n.CreatedAt,
				URL:        n.Status.URL,
			},
		}

		for _, v := range n.Status.MediaAttachments {
			ev.Media = append(ev.Media, v.PreviewURL)
		}
	}

	switch n.Type {
	case "mention":
		if n.Status.InReplyToID != nil {
			ev.Mention = true
			ev.Post.ReplyToAuthor = n.Status.InReplyToAccountID.(string)
			ev.Post.ReplyToID = n.Status.InReplyToID.(string)
		}

	case "reblog":
		ev.Forward = true
		ev.Post.Author = n.Status.Account.Username
		ev.Post.AuthorName = n.Status.Account.DisplayName
		ev.Post.AuthorURL = n.Status.Account.URL
		// ev.Post.Avatar = n.Status.Account.Avatar
		ev.Post.Actor = n.Account.Username
		ev.Post.ActorName = n.Account.DisplayName

	case "favourite":
		ev.Like = true

		ev.Post.Author = n.Status.Account.Username
		ev.Post.AuthorName = n.Status.Account.DisplayName
		ev.Post.AuthorURL = n.Status.Account.URL
		// ev.Post.Avatar = n.Status.Account.Avatar
		ev.Post.Actor = n.Account.Username
		ev.Post.ActorName = n.Account.DisplayName

	default:
		fmt.Println("Unknown type:", n.Type)
		return
	}

	mod.evchan <- ev
}

func (mod *Account) handleStatus(s *mastodon.Status) {
	ev := accounts.MessageEvent{
		Account: "mastodon",
		Name:    "post",
		Post: accounts.Post{
			MessageID:  string(s.ID),
			Body:       parseBody(s.Content),
			Author:     s.Account.Acct,
			AuthorName: s.Account.DisplayName,
			AuthorURL:  s.Account.URL,
			Avatar:     s.Account.Avatar,
			CreatedAt:  s.CreatedAt,
			URL:        s.URL,
		},
	}

	for _, v := range s.MediaAttachments {
		ev.Media = append(ev.Media, v.PreviewURL)
	}

	if s.Reblog != nil {
		ev.Forward = true

		for _, v := range s.Reblog.MediaAttachments {
			ev.Media = append(ev.Media, v.PreviewURL)
		}

		ev.Post.URL = s.Reblog.URL
		ev.Post.Author = s.Reblog.Account.Username
		ev.Post.AuthorName = s.Reblog.Account.DisplayName
		ev.Post.AuthorURL = s.Reblog.Account.URL
		ev.Post.Actor = s.Account.DisplayName
		ev.Post.ActorName = s.Account.Username
	}

	mod.evchan <- ev
}

func (mod *Account) handleStreamEvent(item interface{}) {
	spw := &spew.ConfigState{Indent: "  ", DisableCapacities: true, DisablePointerAddresses: true}
	log.Println("Message received:", spw.Sdump(item))

	switch e := item.(type) {
	case *mastodon.NotificationEvent:
		mod.handleNotification(e.Notification)

	case *mastodon.UpdateEvent:
		mod.handleStatus(e.Status)
	}
}

func (mod *Account) handleStream() {
	timeline, err := mod.client.StreamingUser(context.Background())
	if err != nil {
		panic(err)
	}

	for {
		select {
		case <-mod.SigChan:
			return
		case item := <-timeline:
			mod.handleStreamEvent(item)
		}
	}
}
