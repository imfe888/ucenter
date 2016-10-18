package ucenter

import (
	"crypto/md5"
	"fmt"
)

func getUserByName(name string) (*UserInfo, error) {
	sql := "select * from " + Config.UserTableName + " where user_name = ?"
	rows, err := db.Query(sql, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {

		var u UserInfo
		if err = rows.Scan(&u.ID, &u.Nickname, &u.Password,
			&u.Nickname, &u.Email, &u.Registered,
			&u.RefreshToken, &u.RTokenCreated,
			&u.AccessToken, &u.ATokenCreated,
			&u.PreAccessToken); err == nil {
			return &u, nil
		}
		fmt.Println(err)
	}
	return nil, ErrUserNotExist
}

func createUser(user UserInfo) error {
	password := md5.Sum([]byte(user.Password))
	passwordstr := fmt.Sprintf("%x", password)
	sql := "insert into " + Config.UserTableName + "(user_name, " +
		"user_pass, user_nicename, user_email, user_registered," +
		"refresh_token, rtoken_created, access_token, atoken_created ) " +
		"values(?, ?, ?, ?, now(), '', now(), '', now())"
	_, err := db.Exec(sql, user.UserName, passwordstr, user.Nickname,
		user.Email)
	return err
}

func resetRefreshToken(name string, token string) error {
	sql := "update " + Config.UserTableName + " set refresh_token= ?, " +
		" rtoken_created = now() where user_name=?"
	_, err := db.Exec(sql, token, name)
	if err != nil {
		fmt.Println(err)
	}
	return err
}

func resetAccessToken(name string, token string) error {
	sql := "update " + Config.UserTableName + " set access_token= ?, " +
		" atoken_created = now() where user_name=?"
	_, err := db.Exec(sql, token, name)
	if err != nil {
		fmt.Println(err)
	}
	return err
}

func resetPreAccessToken(name string, token string) error {
	sql := "update " + Config.UserTableName + " set pre_access_token= ?"
	_, err := db.Exec(sql, token)
	return err
}
