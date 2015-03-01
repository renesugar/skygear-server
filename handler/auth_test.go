package handler

import (
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	"github.com/oursky/ourd/auth"
	"github.com/oursky/ourd/oddb"
	"github.com/oursky/ourd/oddb/fs"
	"github.com/oursky/ourd/router"
)

func tempDir() string {
	dir, err := ioutil.TempDir("", "ourd.oddb.handler.auth.test")
	if err != nil {
		panic(err)
	}
	return dir
}

// singleTokenStore implementassigns to and returns itself.
type singleTokenStore auth.Token

func (s *singleTokenStore) Get(accessToken string, token *auth.Token) error {
	*token = auth.Token(*s)
	return nil
}

func (s *singleTokenStore) Put(token *auth.Token) error {
	*s = singleTokenStore(*token)
	return nil
}

// Seems like a memory imlementation of oddb will make tests
// faster and easier

func TestHomeHandler(t *testing.T) {
	req := router.Payload{}
	resp := router.Response{}

	HomeHandler(&req, &resp)
	var s statusResponse

	switch pt := resp.Result.(type) {
	default:
		t.Fatalf("unexpected type %T", pt)
	case statusResponse:
		s = resp.Result.(statusResponse)
	}

	if s.Status != "OK" {
		t.Fatalf("got response %v, want `OK`", s.Status)
	}
}

func TestSignupHandler(t *testing.T) {
	dir := tempDir()
	defer os.RemoveAll(dir)

	conn, err := fs.Open("com.oursky.ourd.handler.auth", dir)
	if err != nil {
		panic(err)
	}

	req := router.Payload{
		Data: map[string]interface{}{
			"user_id":  "userinfoid",
			"email":    "john.doe@example.com",
			"password": "secret",
		},
		DBConn: conn,
	}
	resp := router.Response{}
	tokenStore := singleTokenStore{}
	SignupHandler(&req, &resp, &tokenStore)

	authResp, ok := resp.Result.(authResponse)
	if !ok {
		t.Fatalf("got type = %v, want type authResponse", reflect.TypeOf(resp.Result))
	}

	if authResp.UserID != "userinfoid" {
		t.Fatalf("got authResp.UserID = %v, want userinfoid", authResp.UserID)
	}

	if authResp.AccessToken == "" {
		t.Fatal("got authResp.AccessToken, want non-empty value")
	}

	token := auth.Token(tokenStore)
	if token.UserInfoID != "userinfoid" {
		t.Fatalf("got token.UserInfoID = %v, want userinfoid", token.UserInfoID)
	}

	if token.AccessToken == "" {
		t.Fatal("got empty token.AccessToken, want non-empty value")
	}
}

func TestSignupHandlerDuplicated(t *testing.T) {
	dir := tempDir()
	defer os.RemoveAll(dir)

	conn, err := fs.Open("com.oursky.ourd.handler.auth", dir)
	if err != nil {
		panic(err)
	}

	userinfo := oddb.NewUserInfo("userinfoid", "john.doe@example.com", "secret")
	conn.CreateUser(&userinfo)

	req := router.Payload{
		Data: map[string]interface{}{
			"user_id":  "userinfoid",
			"email":    "john.doe@example.com",
			"password": "secret",
		},
		DBConn: conn,
	}
	resp := router.Response{}
	tokenStore := singleTokenStore{}
	SignupHandler(&req, &resp, &tokenStore)

	errorResponse, ok := resp.Result.(genericError)
	if !ok {
		t.Fatalf("got type = %v, want type genericError", reflect.TypeOf(resp.Result))
	}

	if errorResponse.Code != 101 {
		t.Fatalf("got errorResponse.Code = %v, want 101", errorResponse.Code)
	}
}

func TestLoginHandler(t *testing.T) {
	dir := tempDir()
	defer os.RemoveAll(dir)

	conn, err := fs.Open("com.oursky.ourd.handler.auth", dir)
	if err != nil {
		panic(err)
	}

	req := router.Payload{
		Data:   make(map[string]interface{}),
		DBConn: conn,
	}
	req.Data["email"] = "rick.mak@gmail.com"
	resp := router.Response{}

	tokenStore := singleTokenStore{}
	LoginHandler(&req, &resp, &tokenStore)

	var s authResponse

	switch pt := resp.Result.(type) {
	default:
		t.Fatalf("unexpected type %T", pt)
	case authResponse:
		s = resp.Result.(authResponse)
	}

	if s.UserID != "rickmak-oursky" {
		t.Fatalf("UserId mismatch, expecting `rickmak-oursky`, got %v",
			s.UserID)
	}
}