package server

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/drone/drone/common"
	"github.com/drone/drone/datastore"
	"github.com/drone/drone/eventbus"
	"github.com/drone/drone/queue"
	"github.com/drone/drone/remote"
	"github.com/drone/drone/runner"
	"github.com/drone/drone/server/session"
	"github.com/drone/drone/settings"
)

func SetQueue(q queue.Queue) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("queue", q)
		c.Next()
	}
}

func ToQueue(c *gin.Context) queue.Queue {
	v, ok := c.Get("queue")
	if !ok {
		return nil
	}
	return v.(queue.Queue)
}

func SetBus(r eventbus.Bus) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("eventbus", r)
		c.Next()
	}
}

func ToBus(c *gin.Context) eventbus.Bus {
	v, ok := c.Get("eventbus")
	if !ok {
		return nil
	}
	return v.(eventbus.Bus)
}

func ToRemote(c *gin.Context) remote.Remote {
	v, ok := c.Get("remote")
	if !ok {
		return nil
	}
	return v.(remote.Remote)
}

func SetRemote(r remote.Remote) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("remote", r)
		c.Next()
	}
}

func ToRunner(c *gin.Context) runner.Runner {
	v, ok := c.Get("runner")
	if !ok {
		return nil
	}
	return v.(runner.Runner)
}

func SetRunner(r runner.Runner) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("runner", r)
		c.Next()
	}
}

func ToSettings(c *gin.Context) *settings.Settings {
	v, ok := c.Get("settings")
	if !ok {
		return nil
	}
	return v.(*settings.Settings)
}

func SetSettings(s *settings.Settings) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("settings", s)
		c.Next()
	}
}

func ToPerm(c *gin.Context) *common.Perm {
	v, ok := c.Get("perm")
	if !ok {
		return nil
	}
	return v.(*common.Perm)
}

func ToUser(c *gin.Context) *common.User {
	v, ok := c.Get("user")
	if !ok {
		return nil
	}
	return v.(*common.User)
}

func ToRepo(c *gin.Context) *common.Repo {
	v, ok := c.Get("repo")
	if !ok {
		return nil
	}
	return v.(*common.Repo)
}

func ToDatastore(c *gin.Context) datastore.Datastore {
	return c.MustGet("datastore").(datastore.Datastore)
}

func ToSession(c *gin.Context) session.Session {
	return c.MustGet("session").(session.Session)
}

func SetDatastore(ds datastore.Datastore) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("datastore", ds)
		c.Next()
	}
}

func SetSession(s session.Session) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("session", s)
		c.Next()
	}
}

func SetUser(s session.Session) gin.HandlerFunc {
	return func(c *gin.Context) {
		ds := ToDatastore(c)
		token := s.GetLogin(c.Request)
		if token == nil || len(token.Login) == 0 {
			c.Next()
			return
		}

		user, err := ds.UserLogin(token.Login)
		if err == nil {
			c.Set("user", user)
		}

		// if session token we can proceed, otherwise
		// we should validate the token hasn't been revoked
		switch token.Kind {
		case common.TokenSess:
			c.Next()
			return
		}

		// to verify the token we fetch from the datastore
		// and check to see if the token issued date matches
		// what we found in the jwt (in case the label is re-used)
		t, err := ds.TokenLabel(user, token.Label)
		if err != nil || t.Issued != token.Issued {
			c.AbortWithStatus(403)
			return
		}

		c.Next()
	}
}

func SetRepo() gin.HandlerFunc {
	return func(c *gin.Context) {
		ds := ToDatastore(c)
		u := ToUser(c)
		owner := c.Params.ByName("owner")
		name := c.Params.ByName("name")
		r, err := ds.RepoName(owner, name)
		switch {
		case err != nil && u != nil:
			c.Fail(404, err)
			return
		case err != nil && u == nil:
			c.Fail(401, err)
			return
		}
		c.Set("repo", r)
		c.Next()
	}
}

func SetPerm() gin.HandlerFunc {
	return func(c *gin.Context) {
		remote := ToRemote(c)
		user := ToUser(c)
		repo := ToRepo(c)
		perm := perms(remote, user, repo)
		c.Set("perm", perm)
		c.Next()
	}
}

func MustUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := ToUser(c)
		if u == nil {
			c.AbortWithStatus(401)
		} else {
			c.Set("user", u)
			c.Next()
		}
	}
}

func MustAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := ToUser(c)
		if u == nil {
			c.AbortWithStatus(401)
		} else if !u.Admin {
			c.AbortWithStatus(403)
		} else {
			c.Set("user", u)
			c.Next()
		}
	}
}

func MustAgent() gin.HandlerFunc {
	return func(c *gin.Context) {
		sess := ToSession(c)
		token := sess.GetLogin(c.Request)
		if token == nil {
			c.AbortWithStatus(401)
			return
		} else if token.Kind != common.TokenAgent {
			c.AbortWithStatus(500)
			return
		}
		c.Next()
	}
}

func CheckPull() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := ToUser(c)
		m := ToPerm(c)

		switch {
		case u == nil && m == nil:
			c.AbortWithStatus(401)
		case u == nil && m.Pull == false:
			c.AbortWithStatus(401)
		case u != nil && m.Pull == false:
			c.AbortWithStatus(404)
		default:
			c.Next()
		}
	}
}

func CheckPush() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case "GET", "OPTIONS":
			c.Next()
			return
		}

		u := ToUser(c)
		m := ToPerm(c)

		switch {
		case u == nil && m.Push == false:
			c.AbortWithStatus(401)
		case u != nil && m.Push == false:
			c.AbortWithStatus(404)
		default:
			c.Next()
		}
	}
}

func SetHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {

		c.Writer.Header().Add("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Add("X-Frame-Options", "DENY")
		c.Writer.Header().Add("X-Content-Type-Options", "nosniff")
		c.Writer.Header().Add("X-XSS-Protection", "1; mode=block")
		c.Writer.Header().Add("Cache-Control", "no-cache")
		c.Writer.Header().Add("Cache-Control", "no-store")
		c.Writer.Header().Add("Cache-Control", "max-age=0")
		c.Writer.Header().Add("Cache-Control", "must-revalidate")
		c.Writer.Header().Add("Cache-Control", "value")
		c.Writer.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		c.Writer.Header().Set("Expires", "Thu, 01 Jan 1970 00:00:00 GMT")
		if c.Request.TLS != nil {
			c.Writer.Header().Add("Strict-Transport-Security", "max-age=31536000")
		}

		c.Next()
	}
}
