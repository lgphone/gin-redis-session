package session

import (
	"bytes"
	"encoding/gob"
	"github.com/gin-gonic/gin"
	"github.com/gomodule/redigo/redis"
	"github.com/google/uuid"
	"log"
	"net/http"
	"sync"
	"time"
)

const (
	defaultKey  = "github.com/lgphone/gin-session"
	errorFormat = "[sessions] ERROR! %s\n"
)

type Option struct {
	// redis setting
	Host        string
	Password    string
	DB          int
	MaxActive   int
	MaxIdle     int
	IdleTimeout int
	KeyPrefix   string
	// session setting
	Path       string
	Domain     string
	MaxAge     int // 过期时间
	Secure     bool
	HttpOnly   bool
	CookieName string
}

type redisSessionStore struct {
	option *Option
	pool   *redis.Pool
	rwLock sync.RWMutex
	sid    string
	data   map[string]interface{}
	modify bool
	clear  bool
	w      http.ResponseWriter
}

func (s *redisSessionStore) Get(key string) interface{} {
	if val, ok := s.data[key]; ok {
		return val
	}
	return nil
}

func (s *redisSessionStore) Set(key string, val interface{}) {
	s.data[key] = val
	s.modify = true
}

func (s *redisSessionStore) Del(key string) {
	delete(s.data, key)
	s.modify = true
}

func (s *redisSessionStore) Clear() {
	for k := range s.data {
		delete(s.data, k)
	}
	s.clear = true
}

func (s *redisSessionStore) Save() error {
	cookie := &http.Cookie{
		Name:     s.option.CookieName,
		Value:    s.sid,
		MaxAge:   s.option.MaxAge,
		Domain:   s.option.Domain,
		Path:     s.option.Path,
		HttpOnly: s.option.HttpOnly,
		Secure:   s.option.Secure,
	}
	if s.clear {
		if err := s.delete(); err != nil {
			return err
		}
		http.SetCookie(s.w, cookie)
		return nil
	}
	if s.modify && len(s.data) > 0 {
		if err := s.save(); err != nil {
			return err
		}
		http.SetCookie(s.w, cookie)
	}
	return nil
}

func (s *redisSessionStore) save() error {
	log.Printf("save to redis... ")
	conn := s.pool.Get()
	defer conn.Close()
	if err := conn.Err(); err != nil {
		return err
	}
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	if err := enc.Encode(s.data); err != nil {
		return err
	}
	_, err := conn.Do("SET", s.option.KeyPrefix+s.sid, buf.Bytes(), "EX", s.option.MaxAge)
	return err
}

func (s *redisSessionStore) delete() error {
	log.Printf("delete from redis... ")
	conn := s.pool.Get()
	defer conn.Close()
	if err := conn.Err(); err != nil {
		return err
	}
	_, err := conn.Do("DEL", s.option.KeyPrefix+s.sid)
	return err
}

func (s *redisSessionStore) load() error {
	log.Printf("load from redis... ")
	conn := s.pool.Get()
	defer conn.Close()
	if err := conn.Err(); err != nil {
		return err
	}
	reply, err := redis.Bytes(conn.Do("GET", s.option.KeyPrefix+s.sid))
	if err != nil && err != redis.ErrNil {
		return err
	}
	if len(reply) != 0 {
		dec := gob.NewDecoder(bytes.NewBuffer(reply))
		err = dec.Decode(&s.data)
		if err != nil {
			return err
		}
	}
	return nil
}

type session struct {
	option *Option
	pool   *redis.Pool
}

func Init(option *Option) *session {
	if option.CookieName == "" {
		option.CookieName = "session"
	}
	if option.MaxAge == 0 {
		option.MaxAge = 3600
	}
	if option.KeyPrefix == "" {
		option.KeyPrefix = "c_session:"
	}
	if option.IdleTimeout == 0 {
		option.IdleTimeout = 3600
	}
	if option.Host == "" {
		option.Host = "127.0.0.1:6379"
	}
	return &session{
		option: option,
		pool: &redis.Pool{
			MaxActive:   option.MaxActive,
			MaxIdle:     option.MaxIdle,
			IdleTimeout: time.Duration(option.IdleTimeout) * time.Second,
			Dial: func() (redis.Conn, error) {
				c, err := redis.Dial("tcp", option.Host)
				if err != nil {
					return nil, err
				}
				if option.Password != "" {
					if _, err = c.Do("AUTH", option.Password); err != nil {
						c.Close()
						return nil, err
					}
				}
				if _, err = c.Do("SELECT", option.DB); err != nil {
					c.Close()
					return nil, err
				}
				return c, err
			},
			TestOnBorrow: func(c redis.Conn, t time.Time) error {
				if time.Since(t) < time.Minute {
					return nil
				}
				_, err := c.Do("PING")
				return err
			},
		},
	}
}

func (s *session) GetSession(sid string, w http.ResponseWriter) (*redisSessionStore, error) {
	store := &redisSessionStore{
		option: s.option,
		pool:   s.pool,
		sid:    sid,
		data:   make(map[string]interface{}, 256),
		w:      w,
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *session) CreateSession(w http.ResponseWriter) *redisSessionStore {
	return &redisSessionStore{
		option: s.option,
		pool:   s.pool,
		sid:    getNewUUID(),
		data:   make(map[string]interface{}, 256),
		w:      w,
	}
}

func Middleware(s *session) gin.HandlerFunc {
	return func(c *gin.Context) {
		var store *redisSessionStore
		sid, err := c.Cookie(s.option.CookieName)
		if err != nil {
			store = s.CreateSession(c.Writer)
		} else {
			store, err = s.GetSession(sid, c.Writer)
			if err != nil {
				log.Printf(errorFormat, err)
			}
		}
		c.Set(defaultKey, store)
		c.Next()
	}
}

func getNewUUID() string {
	u, _ := uuid.NewRandom()
	return u.String()
}

func GetSession(c *gin.Context) *redisSessionStore {
	return c.MustGet(defaultKey).(*redisSessionStore)
}
