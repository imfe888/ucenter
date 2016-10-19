package ucenter

import (
	"crypto/md5"
	"database/sql"
	"errors"
	"fmt"
	"time"
	// for mysql driver
	_ "github.com/go-sql-driver/mysql"
)

var (
	// Config configure must initialization before call Init()
	Config = Configure{"", "uc_users", 0, 7 * 24 * 60 * 60, 24 * 60 * 60}

	// inner variable
	db            *sql.DB
	tokenCache    *Cache
	preTokenCache *Cache
	sessionCache  *Cache
)

var (
	// ErrUserExist user has exits for register
	ErrUserExist = errors.New("user name has exist")

	// ErrUserNotExist user has not exist
	ErrUserNotExist = errors.New("user has not exist")

	// ErrParamInvalid param not valid
	ErrParamInvalid = errors.New("param not valid")

	// ErrPwdInvalid password invalid
	ErrPwdInvalid = errors.New("password  invalid")

	// ErrSetRefreshToken set refresh_token error
	ErrSetRefreshToken = errors.New("set refresh_token error")

	// ErrSetAccessToken set access_token error
	ErrSetAccessToken = errors.New("set access_token error")

	// ErrRefreshTokenInvalid refresh token is invalid
	ErrRefreshTokenInvalid = errors.New("refresh token is invalid")

	// ErrAccessTokenInvalid access_token is invalid
	ErrAccessTokenInvalid = errors.New("access_token is invalid")

	// ErrTokenExpired token have expired
	ErrTokenExpired = errors.New("token have expired")

	// ErrTimeParse parse string format to Time error
	ErrTimeParse = errors.New("parse string format to Time error")
)

// Configure configure for data and validation
type Configure struct {
	// MysqlConnStr like root:@/ucenter?charset=utf8
	MysqlConnStr  string
	UserTableName string
	NodeIdentfy   int
	// access_token expires_in
	TokenExpiresIn int
	// session expires_in
	SessionExpiresIn int
}

// UserInfo user basic information
type UserInfo struct {
	ID             int64
	UserName       string
	Nickname       string
	Email          string
	Password       string
	Registered     string
	RefreshToken   string
	RTokenCreated  string
	AccessToken    string
	ATokenCreated  string
	PreAccessToken string
}

// LoginResult Login result
type LoginResult struct {
	AccessToken  string
	RefreshToken string
	Session      string
}

// Init check environment and init settings
// not write in init because of need config
func Init() {
	if len(Config.MysqlConnStr) == 0 {
		fmt.Println("please set config.MysqlConnStr for connect mysql")
		return
	}
	var err error
	db, err = sql.Open("mysql", Config.MysqlConnStr)
	if err != nil {
		fmt.Println(err)
		return
	}

	err = makeSureUserTableExist()
	if err != nil {
		fmt.Println(err)
		return
	}
	tokenCache = &Cache{expire: 2 * 60 * 60} // two hours
	tokenCache.Init()
	preTokenCache = &Cache{expire: 2 * 60 * 60} // two hours
	preTokenCache.Init()
	sessionCache = &Cache{expire: Config.SessionExpiresIn}
	sessionCache.Init()
}

// UserRegister register must have set username and password
func UserRegister(user UserInfo) error {
	if len(user.UserName) == 0 || len(user.Password) == 0 {
		return ErrParamInvalid
	}
	u, _ := getUserByName(user.UserName)
	if u != nil {
		return ErrUserExist
	}
	err := createUser(user)
	if err != nil {
		return err
	}
	return nil
}

// UserLogin  user login, if login succeed will return two token string
// first token : refresh_token
// second token: access_token
func UserLogin(name string, password string) (*LoginResult, error) {

	if len(name) == 0 || len(password) == 0 {
		return nil, ErrParamInvalid
	}
	u, err := getUserByName(name)
	if err != nil {
		return nil, err
	}
	pwd := md5.Sum([]byte(password))
	pwdStr := fmt.Sprintf("%x", pwd)
	if pwdStr != u.Password {
		return nil, ErrPwdInvalid
	}
	refreshToken := GetNewToken()
	err = resetRefreshToken(name, refreshToken)
	if err != nil {
		return nil, ErrSetRefreshToken
	}
	accessToken := GetNewToken()
	err = resetAccessToken(name, accessToken)
	if err != nil {
		return nil, ErrSetAccessToken
	}
	resetPreAccessToken(name, "")

	tokenCache.Set(name, accessToken)
	preTokenCache.Set(name, "")

	session := GetNewToken()
	sessionCache.Set(name, session)

	return &LoginResult{accessToken, refreshToken, session}, nil
}

// CheckAccessToken check user is valid?
// because of access_token maybe check every request in app, so
// need save it in cache used to reduce the load
func CheckAccessToken(name string, accessToken string) error {
	token := tokenCache.Get(name)
	if len(token) > 0 { // have load from database
		if token == accessToken {
			return nil
		}
		preToken := preTokenCache.Get(name)
		if preToken == accessToken {
			return nil
		}
		return ErrAccessTokenInvalid
	}

	u, err := getUserByName(name)
	if err != nil {
		return err
	}
	now := time.Now()
	tokenCreated, err := time.Parse("2006-01-02 15:04:05", u.ATokenCreated)
	if err != nil {
		return ErrTimeParse
	}
	if now.Unix()-tokenCreated.Unix() > int64(Config.TokenExpiresIn) ||
		u.AccessToken == "" {
		// expire_in or kill down
		preTokenCache.Set(name, "nil")
		tokenCache.Set(name, "nil")
		return ErrTokenExpired
	}
	preTokenCache.Set(name, u.PreAccessToken)
	tokenCache.Set(name, u.AccessToken)
	if u.AccessToken != accessToken {
		// pre_access_token is valid in 2 hours
		if now.Unix()-tokenCreated.Unix() < int64(3600*2) {
			if accessToken == u.PreAccessToken {
				return nil
			}
		}
		return ErrAccessTokenInvalid
	}
	return nil
}

// ResetAccessToken reset the access_token by refreshToken
// because of access_token maybe check every request in app, so
// need save it in cache used to reduce the load
func ResetAccessToken(name string, refreshToken string) (string, error) {
	u, err := getUserByName(name)
	if err != nil {
		return "", err
	}
	if u.RefreshToken != refreshToken {
		return "", ErrRefreshTokenInvalid
	}
	err = resetPreAccessToken(name, u.AccessToken)
	if err != nil {
		return "", err
	}
	AccessToken := GetNewToken()
	err = resetAccessToken(name, AccessToken)
	if err != nil {
		return "", ErrSetAccessToken
	}
	tokenCache.Set(name, AccessToken)
	preTokenCache.Set(name, u.AccessToken)
	return AccessToken, nil
}

// CheckSession check session for web site,
// and it will auto refresh session expires_in
func CheckSession(name string, session string) bool {
	s := sessionCache.Get(name)
	if len(s) == 0 || s != session {
		return false
	}
	sessionCache.Set(name, session)
	return true
}

// GetUserInfo get user basic info but not contain authentication information
func GetUserInfo(name string) (*UserInfo, error) {
	u, err := getUserByName(name)
	if err != nil {
		return nil, err
	}
	u.RefreshToken = ""
	u.AccessToken = ""
	u.PreAccessToken = ""
	return u, nil
}

// KillOffLine will delete user token
func KillOffLine(name string) error {
	_, err := getUserByName(name)
	if err != nil {
		return err
	}
	resetRefreshToken(name, "")
	resetAccessToken(name, "")

	tokenCache.Set(name, "nil")
	preTokenCache.Set(name, "nil")
	sessionCache.Set(name, "")

	return nil
}

func makeSureUserTableExist() error {
	// check user table have created
	tables, err := getAllTables()
	if err != nil {
		return err
	}
	findedUserTable := false
	for i := 0; i < len(tables); i++ {
		if Config.UserTableName == tables[i] {
			findedUserTable = true
			break
		}
	}
	if !findedUserTable {
		err := createUserTable()
		if err != nil {
			return err
		}
	}
	return nil
}

func getAllTables() ([]string, error) {
	// 得到所有的分类
	rows, err := db.Query("show tables like '%%'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var table string
		if rows.Scan(&table) == nil {
			tables = append(tables, table)
		}
	}
	return tables, nil
}

// create user table
// pre_access_token: used when refresh access_token, but sometimes app
// can't update right now, so pre_access_token is valid in 2 hours
// session information should not save in database for web site,
// bacause it will change very frequently
func createUserTable() error {
	createStr := "create table " + Config.UserTableName + "(" +
		"ID               bigint(20) unsigned NOT NULL AUTO_INCREMENT," +
		"user_name        varchar(60) NOT NULL DEFAULT ''," +
		"user_pass        varchar(255) NOT NULL DEFAULT ''," +
		"user_nicename    varchar(50) NOT NULL DEFAULT ''," +
		"user_email       varchar(100) NOT NULL DEFAULT ''," +
		"user_registered  datetime NOT NULL DEFAULT CURRENT_TIMESTAMP," +
		"refresh_token    varchar(255) NOT NULL DEFAULT ''," +
		"rtoken_created   datetime NOT NULL DEFAULT CURRENT_TIMESTAMP," +
		"access_token     varchar(255) NOT NULL DEFAULT ''," +
		"atoken_created   datetime NOT NULL DEFAULT CURRENT_TIMESTAMP," +
		"pre_access_token varchar(255) NOT NULL DEFAULT ''," +
		"PRIMARY KEY (`ID`), " +
		"KEY `user_email` (`user_email`)" +
		") ENGINE=InnoDB DEFAULT CHARSET=utf8"
	_, err := db.Exec(createStr)
	if err != nil {
		return err
	}
	return nil
}
