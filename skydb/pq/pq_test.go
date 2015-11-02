package pq

import (
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/lib/pq"
	. "github.com/oursky/skygear/ourtest"
	"github.com/oursky/skygear/skydb"
	. "github.com/smartystreets/goconvey/convey"
)

// NOTE(limouren): postgresql uses this error to signify a non-exist
// schema
func isInvalidSchemaName(err error) bool {
	if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "3F000" {
		return true
	}

	return false
}

func getTestConn(t *testing.T) *conn {
	defaultTo := func(envvar string, value string) {
		if os.Getenv(envvar) == "" {
			os.Setenv(envvar, value)
		}
	}
	defaultTo("PGDATABASE", "skygear_test")
	defaultTo("PGSSLMODE", "disable")
	c, err := Open("com.oursky.skygear", "")
	if err != nil {
		t.Fatal(err)
	}
	return c.(*conn)
}

func cleanupDB(t *testing.T, execori execor) {
	_, err := execori.Exec("DROP SCHEMA app_com_oursky_skygear CASCADE")
	if err != nil && !isInvalidSchemaName(err) {
		t.Fatal(err)
	}
}

func addUser(t *testing.T, c *conn, userid string) {
	_, err := c.Db.Exec("INSERT INTO app_com_oursky_skygear._user (id, password) VALUES ($1, 'somepassword')", userid)
	if err != nil {
		t.Fatal(err)
	}
}

type execor interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}

func insertRow(t *testing.T, db execor, query string, args ...interface{}) {
	result, err := db.Exec(query, args...)
	if err != nil {
		t.Fatal(err)
	}

	n, err := result.RowsAffected()
	if err != nil {
		t.Fatal(err)
	}

	if n != 1 {
		t.Fatalf("got rows affected = %v, want 1", n)
	}
}

func exhaustRows(rows *skydb.Rows, errin error) (records []skydb.Record, err error) {
	if errin != nil {
		err = errin
		return
	}

	for rows.Scan() {
		records = append(records, rows.Record())
	}

	err = rows.Err()
	return
}

func TestUserCRUD(t *testing.T) {
	var c *conn

	Convey("Conn", t, func() {
		c = getTestConn(t)
		defer cleanupDB(t, c.Db)

		userinfo := skydb.UserInfo{
			ID:             "userid",
			Email:          "john.doe@example.com",
			HashedPassword: []byte("$2a$10$RbmNb3Rw.PONA2QTcpjBg.1E00zdSI6dWTUwZi.XC0wZm9OhOEvKO"),
			Auth: skydb.AuthInfo{
				"com.example:johndoe": map[string]interface{}{
					"string": "string",
					"bool":   true,
					"number": float64(1),
				},
			},
		}

		Convey("creates user", func() {
			err := c.CreateUser(&userinfo)
			So(err, ShouldBeNil)

			email := ""
			password := []byte{}
			auth := authInfoValue{}
			err = c.Db.QueryRow("SELECT email, password, auth FROM app_com_oursky_skygear._user WHERE id = 'userid'").
				Scan(&email, &password, &auth)
			So(err, ShouldBeNil)

			So(email, ShouldEqual, "john.doe@example.com")
			So(password, ShouldResemble, []byte("$2a$10$RbmNb3Rw.PONA2QTcpjBg.1E00zdSI6dWTUwZi.XC0wZm9OhOEvKO"))
			So(auth, ShouldResemble, authInfoValue{
				"com.example:johndoe": map[string]interface{}{
					"string": "string",
					"bool":   true,
					"number": float64(1),
				},
			})
		})

		Convey("returns ErrUserDuplicated when user to create already exists", func() {
			err := c.CreateUser(&userinfo)
			So(err, ShouldBeNil)

			err = c.CreateUser(&userinfo)
			So(err, ShouldEqual, skydb.ErrUserDuplicated)
		})

		Convey("gets an existing User", func() {
			err := c.CreateUser(&userinfo)
			So(err, ShouldBeNil)

			fetcheduserinfo := skydb.UserInfo{}
			err = c.GetUser("userid", &fetcheduserinfo)
			So(err, ShouldBeNil)

			So(fetcheduserinfo, ShouldResemble, userinfo)
		})

		Convey("gets an existing User by principal", func() {
			err := c.CreateUser(&userinfo)
			So(err, ShouldBeNil)

			fetcheduserinfo := skydb.UserInfo{}
			err = c.GetUserByPrincipalID("com.example:johndoe", &fetcheduserinfo)
			So(err, ShouldBeNil)

			So(fetcheduserinfo, ShouldResemble, userinfo)
		})

		Convey("returns ErrUserNotFound when the user does not exist", func() {
			err := c.GetUser("userid", (*skydb.UserInfo)(nil))
			So(err, ShouldEqual, skydb.ErrUserNotFound)
		})

		Convey("returns ErrUserNotFound when the user does not exist by principal", func() {
			err := c.GetUserByPrincipalID("com.example:janedoe", (*skydb.UserInfo)(nil))
			So(err, ShouldEqual, skydb.ErrUserNotFound)
		})

		Convey("updates a user", func() {
			err := c.CreateUser(&userinfo)
			So(err, ShouldBeNil)

			userinfo.Email = "jane.doe@example.com"

			err = c.UpdateUser(&userinfo)
			So(err, ShouldBeNil)

			updateduserinfo := userInfo{}
			err = c.Db.Get(&updateduserinfo, "SELECT id, email, password, auth FROM app_com_oursky_skygear._user WHERE id = $1", "userid")
			So(err, ShouldBeNil)
			So(updateduserinfo, ShouldResemble, userInfo{
				ID:             "userid",
				Email:          "jane.doe@example.com",
				HashedPassword: []byte("$2a$10$RbmNb3Rw.PONA2QTcpjBg.1E00zdSI6dWTUwZi.XC0wZm9OhOEvKO"),
				Auth: authInfoValue{
					"com.example:johndoe": map[string]interface{}{
						"string": "string",
						"bool":   true,
						"number": float64(1),
					},
				},
			})
		})

		Convey("query for empty", func() {
			userinfo.Email = ""
			err := c.CreateUser(&userinfo)
			So(err, ShouldBeNil)

			emails := []string{""}
			results, err := c.QueryUser(emails)
			So(err, ShouldBeNil)
			So(len(results), ShouldEqual, 0)
		})

		Convey("query for users", func() {
			err := c.CreateUser(&userinfo)
			So(err, ShouldBeNil)

			userinfo.Email = "jane.doe@example.com"
			userinfo.ID = "userid2"
			So(c.CreateUser(&userinfo), ShouldBeNil)

			emails := []string{"john.doe@example.com", "jane.doe@example.com"}
			results, err := c.QueryUser(emails)
			So(err, ShouldBeNil)

			userids := []string{}
			for _, userinfo := range results {
				userids = append(userids, userinfo.ID)
			}
			So(userids, ShouldContain, "userid")
			So(userids, ShouldContain, "userid2")
		})

		Convey("returns ErrUserNotFound when the user to update does not exist", func() {
			err := c.UpdateUser(&userinfo)
			So(err, ShouldEqual, skydb.ErrUserNotFound)
		})

		Convey("deletes an existing user", func() {
			err := c.CreateUser(&userinfo)
			So(err, ShouldBeNil)

			err = c.DeleteUser("userid")
			So(err, ShouldBeNil)

			placeholder := []byte{}
			err = c.Db.QueryRow("SELECT false FROM app_com_oursky_skygear._user WHERE id = $1", "userid").Scan(&placeholder)
			So(err, ShouldEqual, sql.ErrNoRows)
			So(placeholder, ShouldBeEmpty)
		})

		Convey("returns ErrUserNotFound when the user to delete does not exist", func() {
			err := c.DeleteUser("notexistid")
			So(err, ShouldEqual, skydb.ErrUserNotFound)
		})

		Convey("deletes only the desired user", func() {
			userinfo.ID = "1"
			err := c.CreateUser(&userinfo)
			So(err, ShouldBeNil)

			userinfo.ID = "2"
			err = c.CreateUser(&userinfo)
			So(err, ShouldBeNil)

			count := 0
			c.Db.QueryRow("SELECT COUNT(*) FROM app_com_oursky_skygear._user").Scan(&count)
			So(count, ShouldEqual, 2)

			err = c.DeleteUser("2")
			So(err, ShouldBeNil)

			c.Db.QueryRow("SELECT COUNT(*) FROM app_com_oursky_skygear._user").Scan(&count)
			So(count, ShouldEqual, 1)
		})
	})
}

func TestRelation(t *testing.T) {
	Convey("Conn", t, func() {
		c := getTestConn(t)
		defer cleanupDB(t, c.Db)

		addUser(t, c, "userid")
		addUser(t, c, "friendid")

		Convey("add relation", func() {
			err := c.AddRelation("userid", "friend", "friendid")
			So(err, ShouldBeNil)
		})

		Convey("add a user not exist relation", func() {
			err := c.AddRelation("userid", "friend", "non-exist")
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual, "userID not exist")
		})

		Convey("remove non-exist relation", func() {
			err := c.RemoveRelation("userid", "friend", "friendid")
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual,
				"friend relation not exist {userid} => {friendid}")
		})

		Convey("remove relation", func() {
			err := c.AddRelation("userid", "friend", "friendid")
			So(err, ShouldBeNil)
			err = c.RemoveRelation("userid", "friend", "friendid")
			So(err, ShouldBeNil)
		})
	})
}

func TestDevice(t *testing.T) {
	Convey("Conn", t, func() {
		c := getTestConn(t)
		defer cleanupDB(t, c.Db)

		addUser(t, c, "userid")

		Convey("gets an existing Device", func() {
			device := skydb.Device{
				ID:               "deviceid",
				Type:             "ios",
				Token:            "devicetoken",
				UserInfoID:       "userid",
				LastRegisteredAt: time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC),
			}
			So(c.SaveDevice(&device), ShouldBeNil)

			device = skydb.Device{}
			err := c.GetDevice("deviceid", &device)
			So(err, ShouldBeNil)
			So(device, ShouldResemble, skydb.Device{
				ID:               "deviceid",
				Type:             "ios",
				Token:            "devicetoken",
				UserInfoID:       "userid",
				LastRegisteredAt: time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC),
			})
		})

		Convey("creates a new Device", func() {
			device := skydb.Device{
				ID:               "deviceid",
				Type:             "ios",
				Token:            "devicetoken",
				UserInfoID:       "userid",
				LastRegisteredAt: time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC),
			}

			err := c.SaveDevice(&device)
			So(err, ShouldBeNil)

			var (
				deviceType, token, userInfoID string
				lastRegisteredAt              time.Time
			)
			err = c.Db.QueryRow("SELECT type, token, user_id, last_registered_at FROM app_com_oursky_skygear._device WHERE id = 'deviceid'").
				Scan(&deviceType, &token, &userInfoID, &lastRegisteredAt)
			So(err, ShouldBeNil)
			So(deviceType, ShouldEqual, "ios")
			So(token, ShouldEqual, "devicetoken")
			So(userInfoID, ShouldEqual, "userid")
			So(lastRegisteredAt.Unix(), ShouldEqual, 1136214245)
		})

		Convey("updates an existing Device", func() {
			device := skydb.Device{
				ID:               "deviceid",
				Type:             "ios",
				Token:            "devicetoken",
				UserInfoID:       "userid",
				LastRegisteredAt: time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC),
			}

			err := c.SaveDevice(&device)
			So(err, ShouldBeNil)

			device.Token = "anotherdevicetoken"
			So(c.SaveDevice(&device), ShouldBeNil)

			var (
				deviceType, token, userInfoID string
				lastRegisteredAt              time.Time
			)
			err = c.Db.QueryRow("SELECT type, token, user_id, last_registered_at FROM app_com_oursky_skygear._device WHERE id = 'deviceid'").
				Scan(&deviceType, &token, &userInfoID, &lastRegisteredAt)
			So(err, ShouldBeNil)
			So(deviceType, ShouldEqual, "ios")
			So(token, ShouldEqual, "anotherdevicetoken")
			So(userInfoID, ShouldEqual, "userid")
			So(lastRegisteredAt.Unix(), ShouldEqual, 1136214245)
		})

		Convey("cannot save Device without id", func() {
			device := skydb.Device{
				Type:             "ios",
				Token:            "devicetoken",
				UserInfoID:       "userid",
				LastRegisteredAt: time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC),
			}

			err := c.SaveDevice(&device)
			So(err, ShouldNotBeNil)
		})

		Convey("cannot save Device without type", func() {
			device := skydb.Device{
				ID:               "deviceid",
				Token:            "devicetoken",
				UserInfoID:       "userid",
				LastRegisteredAt: time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC),
			}

			err := c.SaveDevice(&device)
			So(err, ShouldNotBeNil)
		})

		Convey("can save Device without token", func() {
			device := skydb.Device{
				ID:               "deviceid",
				Type:             "pubsub",
				UserInfoID:       "userid",
				LastRegisteredAt: time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC),
			}

			err := c.SaveDevice(&device)
			So(err, ShouldBeNil)
		})

		Convey("cannot save Device without user id", func() {
			device := skydb.Device{
				ID:               "deviceid",
				Type:             "ios",
				Token:            "devicetoken",
				LastRegisteredAt: time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC),
			}

			err := c.SaveDevice(&device)
			So(err, ShouldNotBeNil)
		})

		Convey("cannot save Device without last_registered_at", func() {
			device := skydb.Device{
				ID:         "deviceid",
				Type:       "ios",
				Token:      "devicetoken",
				UserInfoID: "userid",
			}

			err := c.SaveDevice(&device)
			So(err, ShouldNotBeNil)
		})

		Convey("deletes an existing record", func() {
			device := skydb.Device{
				ID:               "deviceid",
				Type:             "ios",
				Token:            "devicetoken",
				UserInfoID:       "userid",
				LastRegisteredAt: time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC),
			}
			So(c.SaveDevice(&device), ShouldBeNil)

			err := c.DeleteDevice("deviceid")
			So(err, ShouldBeNil)

			var count int
			err = c.Db.QueryRow("SELECT COUNT(*) FROM app_com_oursky_skygear._device WHERE id = 'deviceid'").Scan(&count)
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 0)
		})

		Convey("deletes an existing record by token", func() {
			device := skydb.Device{
				ID:               "deviceid",
				Type:             "ios",
				Token:            "devicetoken",
				UserInfoID:       "userid",
				LastRegisteredAt: time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC),
			}
			So(c.SaveDevice(&device), ShouldBeNil)

			err := c.DeleteDeviceByToken("devicetoken", skydb.ZeroTime)
			So(err, ShouldBeNil)

			var count int
			err = c.Db.QueryRow("SELECT COUNT(*) FROM app_com_oursky_skygear._device WHERE id = 'deviceid'").Scan(&count)
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 0)
		})

		Convey("fails to delete an existing record with a later LastRegisteredAt", func() {
			device := skydb.Device{
				ID:               "deviceid",
				Type:             "ios",
				Token:            "devicetoken",
				UserInfoID:       "userid",
				LastRegisteredAt: time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC),
			}
			So(c.SaveDevice(&device), ShouldBeNil)

			err := c.DeleteDeviceByToken("devicetoken", time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC))
			So(err, ShouldEqual, skydb.ErrDeviceNotFound)
		})

		Convey("deletes existing empty records", func() {
			device0 := skydb.Device{
				ID:               "deviceid0",
				Type:             "ios",
				Token:            "DEVICE_TOKEN",
				UserInfoID:       "userid",
				LastRegisteredAt: time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC),
			}
			device1 := skydb.Device{
				ID:               "deviceid1",
				Type:             "ios",
				Token:            "",
				UserInfoID:       "userid",
				LastRegisteredAt: time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC),
			}
			device2 := skydb.Device{
				ID:               "deviceid2",
				Type:             "ios",
				Token:            "",
				UserInfoID:       "userid",
				LastRegisteredAt: time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC),
			}
			So(c.SaveDevice(&device0), ShouldBeNil)
			So(c.SaveDevice(&device1), ShouldBeNil)
			So(c.SaveDevice(&device2), ShouldBeNil)

			err := c.DeleteEmptyDevicesByTime(skydb.ZeroTime)
			So(err, ShouldBeNil)

			var count int
			err = c.Db.QueryRow("SELECT COUNT(*) FROM app_com_oursky_skygear._device").Scan(&count)
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 1)
		})

		Convey("deletes existing empty records before a date", func() {
			device0 := skydb.Device{
				ID:               "deviceid0",
				Type:             "ios",
				Token:            "",
				UserInfoID:       "userid",
				LastRegisteredAt: time.Date(2006, 1, 2, 15, 4, 4, 59, time.UTC),
			}
			device1 := skydb.Device{
				ID:               "deviceid1",
				Type:             "ios",
				Token:            "DEVICE_TOKEN",
				UserInfoID:       "userid",
				LastRegisteredAt: time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC),
			}
			So(c.SaveDevice(&device0), ShouldBeNil)
			So(c.SaveDevice(&device1), ShouldBeNil)

			err := c.DeleteEmptyDevicesByTime(time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC))
			So(err, ShouldBeNil)

			device := skydb.Device{}
			So(c.GetDevice("deviceid0", &device), ShouldEqual, skydb.ErrDeviceNotFound)
			So(c.GetDevice("deviceid1", &device), ShouldBeNil)
			So(device, ShouldResemble, device1)
		})

		Convey("fails to delete an existing record by type with a later LastRegisteredAt", func() {
			device := skydb.Device{
				ID:               "deviceid",
				Type:             "ios",
				Token:            "devicetoken",
				UserInfoID:       "userid",
				LastRegisteredAt: time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC),
			}
			So(c.SaveDevice(&device), ShouldBeNil)

			err := c.DeleteEmptyDevicesByTime(time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC))
			So(err, ShouldEqual, skydb.ErrDeviceNotFound)
		})

		Convey("query devices by user", func() {
			device := skydb.Device{
				ID:               "device",
				Type:             "ios",
				Token:            "devicetoken",
				UserInfoID:       "userid",
				LastRegisteredAt: time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC),
			}
			So(c.SaveDevice(&device), ShouldBeNil)

			devices, err := c.QueryDevicesByUser("userid")
			So(err, ShouldBeNil)
			So(len(devices), ShouldEqual, 1)
			So(devices[0], ShouldResemble, skydb.Device{
				ID:               "device",
				Type:             "ios",
				Token:            "devicetoken",
				UserInfoID:       "userid",
				LastRegisteredAt: time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC),
			})

			devices, err = c.QueryDevicesByUser("nonexistent")
			So(err, ShouldBeNil)
			So(len(devices), ShouldEqual, 0)
		})
	})
}

func TestExtend(t *testing.T) {
	Convey("Extend", t, func() {
		c := getTestConn(t)
		defer cleanupDB(t, c.Db)

		db := c.PublicDB()

		Convey("creates table if not exist", func() {
			err := db.Extend("note", skydb.RecordSchema{
				"content":   skydb.FieldType{Type: skydb.TypeString},
				"noteOrder": skydb.FieldType{Type: skydb.TypeNumber},
				"createdAt": skydb.FieldType{Type: skydb.TypeDateTime},
			})
			So(err, ShouldBeNil)

			// verify with an insert
			result, err := c.Db.Exec(
				`INSERT INTO app_com_oursky_skygear."note" ` +
					`(_id, _database_id, _owner_id, _created_at, _created_by, _updated_at, _updated_by, "content", "noteOrder", "createdAt") ` +
					`VALUES (1, 1, 1, '1988-02-06', 'creator', '1988-02-06', 'updater', 'some content', 2, '1988-02-06')`)
			So(err, ShouldBeNil)

			i, err := result.RowsAffected()
			So(err, ShouldBeNil)
			So(i, ShouldEqual, 1)
		})

		Convey("REGRESSION #277: creates table with `:`", func() {
			err := db.Extend("table:name", nil)
			So(err, ShouldBeNil)
		})

		Convey("creates table with JSON field", func() {
			err := db.Extend("note", skydb.RecordSchema{
				"tags": skydb.FieldType{Type: skydb.TypeJSON},
			})
			So(err, ShouldBeNil)

			result, err := c.Db.Exec(
				`INSERT INTO app_com_oursky_skygear."note" ` +
					`(_id, _database_id, _owner_id, _created_at, _created_by, _updated_at, _updated_by, "tags") ` +
					`VALUES (1, 1, 1, '1988-02-06', 'creator', '1988-02-06', 'updater', '["tag0", "tag1"]')`)
			So(err, ShouldBeNil)

			i, err := result.RowsAffected()
			So(err, ShouldBeNil)
			So(i, ShouldEqual, 1)
		})

		Convey("creates table with asset", func() {
			err := db.Extend("note", skydb.RecordSchema{
				"image": skydb.FieldType{Type: skydb.TypeAsset},
			})
			So(err, ShouldBeNil)
		})

		Convey("creates table with multiple assets", func() {
			err := db.Extend("note", skydb.RecordSchema{
				"image0": skydb.FieldType{Type: skydb.TypeAsset},
			})
			So(err, ShouldBeNil)
			err = db.Extend("note", skydb.RecordSchema{
				"image1": skydb.FieldType{Type: skydb.TypeAsset},
			})
			So(err, ShouldBeNil)
		})

		Convey("creates table with reference", func() {
			err := db.Extend("collection", skydb.RecordSchema{
				"name": skydb.FieldType{Type: skydb.TypeString},
			})
			So(err, ShouldBeNil)
			err = db.Extend("note", skydb.RecordSchema{
				"content": skydb.FieldType{Type: skydb.TypeString},
				"collection": skydb.FieldType{
					Type:          skydb.TypeReference,
					ReferenceType: "collection",
				},
			})
			So(err, ShouldBeNil)
		})

		Convey("REGRESSION #318: creates table with `:` with reference", func() {
			err := db.Extend("colon:fever", skydb.RecordSchema{
				"name": skydb.FieldType{Type: skydb.TypeString},
			})
			So(err, ShouldBeNil)
			err = db.Extend("note", skydb.RecordSchema{
				"content": skydb.FieldType{Type: skydb.TypeString},
				"colon:fever": skydb.FieldType{
					Type:          skydb.TypeReference,
					ReferenceType: "colon:fever",
				},
			})
			So(err, ShouldBeNil)
		})

		Convey("creates table with location", func() {
			err := db.Extend("photo", skydb.RecordSchema{
				"location": skydb.FieldType{Type: skydb.TypeLocation},
			})
			So(err, ShouldBeNil)
		})

		Convey("error if creates table with reference not exist", func() {
			err := db.Extend("note", skydb.RecordSchema{
				"content": skydb.FieldType{Type: skydb.TypeString},
				"tag": skydb.FieldType{
					Type:          skydb.TypeReference,
					ReferenceType: "tag",
				},
			})
			So(err, ShouldNotBeNil)
		})

		Convey("adds new column if table already exist", func() {
			err := db.Extend("note", skydb.RecordSchema{
				"content":   skydb.FieldType{Type: skydb.TypeString},
				"noteOrder": skydb.FieldType{Type: skydb.TypeNumber},
				"createdAt": skydb.FieldType{Type: skydb.TypeDateTime},
			})
			So(err, ShouldBeNil)

			err = db.Extend("note", skydb.RecordSchema{
				"createdAt": skydb.FieldType{Type: skydb.TypeDateTime},
				"dirty":     skydb.FieldType{Type: skydb.TypeBoolean},
			})
			So(err, ShouldBeNil)

			// verify with an insert
			result, err := c.Db.Exec(
				`INSERT INTO app_com_oursky_skygear."note" ` +
					`(_id, _database_id, _owner_id, _created_at, _created_by, _updated_at, _updated_by, "content", "noteOrder", "createdAt", "dirty") ` +
					`VALUES (1, 1, 1, '1988-02-06', 'creator', '1988-02-06', 'updater', 'some content', 2, '1988-02-06', TRUE)`)
			So(err, ShouldBeNil)

			i, err := result.RowsAffected()
			So(err, ShouldBeNil)
			So(i, ShouldEqual, 1)
		})

		Convey("errors if conflict with existing column type", func() {
			err := db.Extend("note", skydb.RecordSchema{
				"content":   skydb.FieldType{Type: skydb.TypeString},
				"noteOrder": skydb.FieldType{Type: skydb.TypeNumber},
				"createdAt": skydb.FieldType{Type: skydb.TypeDateTime},
			})
			So(err, ShouldBeNil)

			err = db.Extend("note", skydb.RecordSchema{
				"content":   skydb.FieldType{Type: skydb.TypeNumber},
				"createdAt": skydb.FieldType{Type: skydb.TypeDateTime},
				"dirty":     skydb.FieldType{Type: skydb.TypeNumber},
			})
			So(err.Error(), ShouldEqual, "conflicting schema {TypeString  %!s(*skydb.Expression=<nil>)} => {TypeNumber  %!s(*skydb.Expression=<nil>)}")
		})
	})
}

func TestGet(t *testing.T) {
	Convey("Database", t, func() {
		c := getTestConn(t)
		defer cleanupDB(t, c.Db)

		db := c.PrivateDB("getuser")
		So(db.Extend("record", skydb.RecordSchema{
			"string":   skydb.FieldType{Type: skydb.TypeString},
			"number":   skydb.FieldType{Type: skydb.TypeNumber},
			"datetime": skydb.FieldType{Type: skydb.TypeDateTime},
			"boolean":  skydb.FieldType{Type: skydb.TypeBoolean},
		}), ShouldBeNil)

		insertRow(t, c.Db, `INSERT INTO app_com_oursky_skygear."record" `+
			`(_database_id, _id, _owner_id, _created_at, _created_by, _updated_at, _updated_by, "string", "number", "datetime", "boolean") `+
			`VALUES ('getuser', 'id0', 'getuser', '1988-02-06', 'getuser', '1988-02-06', 'getuser', 'string', 1, '1988-02-06', TRUE)`)
		insertRow(t, c.Db, `INSERT INTO app_com_oursky_skygear."record" `+
			`(_database_id, _id, _owner_id, _created_at, _created_by, _updated_at, _updated_by, "string", "number", "datetime", "boolean") `+
			`VALUES ('getuser', 'id1', 'getuser', '1988-02-06', 'getuser', '1988-02-06', 'getuser', 'string', 1, '1988-02-06', TRUE)`)

		Convey("gets an existing record from database", func() {
			record := skydb.Record{}
			err := db.Get(skydb.NewRecordID("record", "id1"), &record)
			So(err, ShouldBeNil)

			So(record.ID, ShouldResemble, skydb.NewRecordID("record", "id1"))
			So(record.DatabaseID, ShouldResemble, "getuser")
			So(record.OwnerID, ShouldResemble, "getuser")
			So(record.CreatorID, ShouldResemble, "getuser")
			So(record.UpdaterID, ShouldResemble, "getuser")
			So(record.Data["string"], ShouldEqual, "string")
			So(record.Data["number"], ShouldEqual, 1)
			So(record.Data["boolean"], ShouldEqual, true)

			So(record.CreatedAt, ShouldResemble, time.Date(1988, 2, 6, 0, 0, 0, 0, time.UTC))
			So(record.UpdatedAt, ShouldResemble, time.Date(1988, 2, 6, 0, 0, 0, 0, time.UTC))
			So(record.Data["datetime"].(time.Time), ShouldResemble, time.Date(1988, 2, 6, 0, 0, 0, 0, time.UTC))
		})

		Convey("errors if gets a non-existing record", func() {
			record := skydb.Record{}
			err := db.Get(skydb.NewRecordID("record", "notexistid"), &record)
			So(err, ShouldEqual, skydb.ErrRecordNotFound)
		})
	})
}

func TestSave(t *testing.T) {
	var c *conn
	Convey("Database", t, func() {
		c = getTestConn(t)
		defer cleanupDB(t, c.Db)

		db := c.PublicDB()
		So(db.Extend("note", skydb.RecordSchema{
			"content":   skydb.FieldType{Type: skydb.TypeString},
			"number":    skydb.FieldType{Type: skydb.TypeNumber},
			"timestamp": skydb.FieldType{Type: skydb.TypeDateTime},
		}), ShouldBeNil)

		record := skydb.Record{
			ID:        skydb.NewRecordID("note", "someid"),
			OwnerID:   "user_id",
			CreatedAt: time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC),
			CreatorID: "creator",
			UpdatedAt: time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC),
			UpdaterID: "updater",
			Data: map[string]interface{}{
				"content":   "some content",
				"number":    float64(1),
				"timestamp": time.Date(1988, 2, 6, 1, 1, 1, 1, time.UTC),
			},
		}

		Convey("creates record if it doesn't exist", func() {
			err := db.Save(&record)
			So(err, ShouldBeNil)
			So(record.DatabaseID, ShouldEqual, "")

			var (
				content   string
				number    float64
				timestamp time.Time
				ownerID   string
			)
			err = c.Db.QueryRow(
				"SELECT content, number, timestamp, _owner_id "+
					"FROM app_com_oursky_skygear.note WHERE _id = 'someid' and _database_id = ''").
				Scan(&content, &number, &timestamp, &ownerID)
			So(err, ShouldBeNil)
			So(content, ShouldEqual, "some content")
			So(number, ShouldEqual, float64(1))
			So(timestamp.In(time.UTC), ShouldResemble, time.Date(1988, 2, 6, 1, 1, 1, 0, time.UTC))
			So(ownerID, ShouldEqual, "user_id")
		})

		Convey("updates record if it already exists", func() {
			err := db.Save(&record)
			So(err, ShouldBeNil)
			So(record.DatabaseID, ShouldEqual, "")

			record.Set("content", "more content")
			err = db.Save(&record)
			So(err, ShouldBeNil)

			var content string
			err = c.Db.QueryRow("SELECT content FROM app_com_oursky_skygear.note WHERE _id = 'someid' and _database_id = ''").
				Scan(&content)
			So(err, ShouldBeNil)
			So(content, ShouldEqual, "more content")
		})

		Convey("error if saving with recordid already taken by other user", func() {
			ownerDB := c.PrivateDB("ownerid")
			err := ownerDB.Save(&record)
			So(err, ShouldBeNil)
			otherDB := c.PrivateDB("otheruserid")
			err = otherDB.Save(&record)
			// FIXME: Wrap me with skydb.ErrXXX
			So(err, ShouldNotBeNil)
		})

		Convey("ignore Record.DatabaseID when saving", func() {
			record.DatabaseID = "someuserid"
			err := db.Save(&record)
			So(err, ShouldBeNil)
			So(record.DatabaseID, ShouldEqual, "")

			var count int
			err = c.Db.QueryRowx("SELECT count(*) FROM app_com_oursky_skygear.note WHERE _id = 'someid' and _database_id = 'someuserid'").
				Scan(&count)
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 0)
		})

		Convey("REGRESSION: update record with attribute having capital letters", func() {
			So(db.Extend("note", skydb.RecordSchema{
				"noteOrder": skydb.FieldType{Type: skydb.TypeNumber},
			}), ShouldBeNil)

			record = skydb.Record{
				ID:      skydb.NewRecordID("note", "1"),
				OwnerID: "user_id",
				Data: map[string]interface{}{
					"noteOrder": 1,
				},
			}

			ShouldBeNil(db.Save(&record))

			record.Data["noteOrder"] = 2
			ShouldBeNil(db.Save(&record))

			var noteOrder int
			err := c.Db.QueryRow(`SELECT "noteOrder" FROM app_com_oursky_skygear.note WHERE _id = '1' and _database_id = ''`).
				Scan(&noteOrder)
			So(err, ShouldBeNil)
			So(noteOrder, ShouldEqual, 2)
		})

		Convey("errors if OwnerID not set", func() {
			record.OwnerID = ""
			err := db.Save(&record)
			So(err.Error(), ShouldEndWith, "got empty OwnerID")
		})

		Convey("ignore OwnerID when update", func() {
			err := db.Save(&record)
			So(err, ShouldBeNil)

			record.OwnerID = "user_id2"
			So(err, ShouldBeNil)

			var ownerID string
			err = c.Db.QueryRow(`SELECT "_owner_id" FROM app_com_oursky_skygear.note WHERE _id = 'someid' and _database_id = ''`).
				Scan(&ownerID)
			So(ownerID, ShouldEqual, "user_id")
		})
	})
}

func TestACL(t *testing.T) {
	Convey("Database", t, func() {
		c := getTestConn(t)
		defer cleanupDB(t, c.Db)

		db := c.PublicDB()
		So(db.Extend("note", nil), ShouldBeNil)

		record := skydb.Record{
			ID:      skydb.NewRecordID("note", "1"),
			OwnerID: "someuserid",
			ACL:     nil,
		}

		Convey("saves public access correctly", func() {
			err := db.Save(&record)

			So(err, ShouldBeNil)

			var b []byte
			err = c.Db.QueryRow(`SELECT _access FROM app_com_oursky_skygear.note WHERE _id = '1'`).
				Scan(&b)
			So(err, ShouldBeNil)
			So(b, ShouldResemble, []byte(nil))
		})
	})
}

func TestJSON(t *testing.T) {
	Convey("Database", t, func() {
		c := getTestConn(t)
		defer cleanupDB(t, c.Db)

		db := c.PublicDB()
		So(db.Extend("note", skydb.RecordSchema{
			"jsonfield": skydb.FieldType{Type: skydb.TypeJSON},
		}), ShouldBeNil)

		Convey("fetch record with json field", func() {
			So(db.Extend("record", skydb.RecordSchema{
				"array":      skydb.FieldType{Type: skydb.TypeJSON},
				"dictionary": skydb.FieldType{Type: skydb.TypeJSON},
			}), ShouldBeNil)

			insertRow(t, c.Db, `INSERT INTO app_com_oursky_skygear."record" `+
				`(_database_id, _id, _owner_id, _created_at, _created_by, _updated_at, _updated_by, "array", "dictionary") `+
				`VALUES ('', 'id', '', '0001-01-01 00:00:00', '', '0001-01-01 00:00:00', '', '[1, "string", true]', '{"number": 0, "string": "value", "bool": false}')`)

			var record skydb.Record
			err := db.Get(skydb.NewRecordID("record", "id"), &record)
			So(err, ShouldBeNil)

			So(record, ShouldResemble, skydb.Record{
				ID: skydb.NewRecordID("record", "id"),
				Data: map[string]interface{}{
					"array": []interface{}{float64(1), "string", true},
					"dictionary": map[string]interface{}{
						"number": float64(0),
						"string": "value",
						"bool":   false,
					},
				},
			})
		})

		Convey("saves record field with array", func() {
			record := skydb.Record{
				ID:      skydb.NewRecordID("note", "1"),
				OwnerID: "user_id",
				Data: map[string]interface{}{
					"jsonfield": []interface{}{0.0, "string", true},
				},
			}

			So(db.Save(&record), ShouldBeNil)

			var jsonBytes []byte
			err := c.Db.QueryRow(`SELECT jsonfield FROM app_com_oursky_skygear.note WHERE _id = '1' and _database_id = ''`).
				Scan(&jsonBytes)
			So(err, ShouldBeNil)
			So(jsonBytes, ShouldEqualJSON, `[0, "string", true]`)
		})

		Convey("saves record field with dictionary", func() {
			record := skydb.Record{
				ID:      skydb.NewRecordID("note", "1"),
				OwnerID: "user_id",
				Data: map[string]interface{}{
					"jsonfield": map[string]interface{}{
						"number": float64(1),
						"string": "",
						"bool":   false,
					},
				},
			}

			So(db.Save(&record), ShouldBeNil)

			var jsonBytes []byte
			err := c.Db.QueryRow(`SELECT jsonfield FROM app_com_oursky_skygear.note WHERE _id = '1' and _database_id = ''`).
				Scan(&jsonBytes)
			So(err, ShouldBeNil)
			So(jsonBytes, ShouldEqualJSON, `{"number": 1, "string": "", "bool": false}`)
		})
	})
}

func TestRecordAssetField(t *testing.T) {
	Convey("Record Asset", t, func() {
		c := getTestConn(t)
		defer cleanupDB(t, c.Db)

		So(c.SaveAsset(&skydb.Asset{
			Name:        "picture.png",
			ContentType: "image/png",
			Size:        1,
		}), ShouldBeNil)

		db := c.PublicDB()
		So(db.Extend("note", skydb.RecordSchema{
			"image": skydb.FieldType{Type: skydb.TypeAsset},
		}), ShouldBeNil)

		Convey("can be associated", func() {
			err := db.Save(&skydb.Record{
				ID: skydb.NewRecordID("note", "id"),
				Data: map[string]interface{}{
					"image": skydb.Asset{Name: "picture.png"},
				},
				OwnerID: "user_id",
			})
			So(err, ShouldBeNil)
		})

		Convey("errors when associated with non-existing asset", func() {
			err := db.Save(&skydb.Record{
				ID: skydb.NewRecordID("note", "id"),
				Data: map[string]interface{}{
					"image": skydb.Asset{Name: "notexist.png"},
				},
				OwnerID: "user_id",
			})
			So(err, ShouldNotBeNil)
		})

		Convey("REGRESSION #229: can be fetched", func() {
			So(db.Save(&skydb.Record{
				ID: skydb.NewRecordID("note", "id"),
				Data: map[string]interface{}{
					"image": skydb.Asset{Name: "picture.png"},
				},
				OwnerID: "user_id",
			}), ShouldBeNil)

			var record skydb.Record
			err := db.Get(skydb.NewRecordID("note", "id"), &record)
			So(err, ShouldBeNil)
			So(record, ShouldResemble, skydb.Record{
				ID: skydb.NewRecordID("note", "id"),
				Data: map[string]interface{}{
					"image": skydb.Asset{Name: "picture.png"},
				},
				OwnerID: "user_id",
			})
		})
	})
}

func TestRecordLocationField(t *testing.T) {
	Convey("Database", t, func() {
		c := getTestConn(t)
		defer cleanupDB(t, c.Db)

		db := c.PublicDB()
		So(db.Extend("photo", skydb.RecordSchema{
			"location": skydb.FieldType{Type: skydb.TypeLocation},
		}), ShouldBeNil)

		Convey("saves & load location field", func() {
			err := db.Save(&skydb.Record{
				ID: skydb.NewRecordID("photo", "1"),
				Data: map[string]interface{}{
					"location": skydb.NewLocation(1, 2),
				},
				OwnerID: "userid",
			})

			So(err, ShouldBeNil)

			record := skydb.Record{}
			err = db.Get(skydb.NewRecordID("photo", "1"), &record)
			So(err, ShouldBeNil)
			So(record, ShouldResemble, skydb.Record{
				ID: skydb.NewRecordID("photo", "1"),
				Data: map[string]interface{}{
					"location": skydb.NewLocation(1, 2),
				},
				OwnerID: "userid",
			})
		})
	})
}

func TestDelete(t *testing.T) {
	var c *conn
	Convey("Database", t, func() {
		c = getTestConn(t)
		defer cleanupDB(t, c.Db)

		db := c.PrivateDB("userid")

		So(db.Extend("note", skydb.RecordSchema{
			"content": skydb.FieldType{Type: skydb.TypeString},
		}), ShouldBeNil)

		record := skydb.Record{
			ID:      skydb.NewRecordID("note", "someid"),
			OwnerID: "user_id",
			Data: map[string]interface{}{
				"content": "some content",
			},
		}

		Convey("deletes existing record", func() {
			err := db.Save(&record)
			So(err, ShouldBeNil)

			err = db.Delete(skydb.NewRecordID("note", "someid"))
			So(err, ShouldBeNil)

			err = db.(*database).Db.QueryRowx("SELECT * FROM app_com_oursky_skygear.note WHERE _id = 'someid' AND _database_id = 'userid'").Scan((*string)(nil))
			So(err, ShouldEqual, sql.ErrNoRows)
		})

		Convey("returns ErrRecordNotFound when record to delete doesn't exist", func() {
			err := db.Delete(skydb.NewRecordID("note", "notexistid"))
			So(err, ShouldEqual, skydb.ErrRecordNotFound)
		})

		Convey("return ErrRecordNotFound when deleting other user record", func() {
			err := db.Save(&record)
			So(err, ShouldBeNil)
			otherDB := c.PrivateDB("otheruserid")
			err = otherDB.Delete(skydb.NewRecordID("note", "someid"))
			So(err, ShouldEqual, skydb.ErrRecordNotFound)
		})
	})
}

func TestQuery(t *testing.T) {
	Convey("Database", t, func() {
		c := getTestConn(t)
		defer cleanupDB(t, c.Db)

		// fixture
		record1 := skydb.Record{
			ID:      skydb.NewRecordID("note", "id1"),
			OwnerID: "user_id",
			Data: map[string]interface{}{
				"noteOrder": float64(1),
				"content":   "Hello World",
			},
		}
		record2 := skydb.Record{
			ID:      skydb.NewRecordID("note", "id2"),
			OwnerID: "user_id",
			Data: map[string]interface{}{
				"noteOrder": float64(2),
				"content":   "Bye World",
			},
		}
		record3 := skydb.Record{
			ID:      skydb.NewRecordID("note", "id3"),
			OwnerID: "user_id",
			Data: map[string]interface{}{
				"noteOrder": float64(3),
				"content":   "Good Hello",
			},
		}

		db := c.PrivateDB("userid")
		So(db.Extend("note", skydb.RecordSchema{
			"noteOrder": skydb.FieldType{Type: skydb.TypeNumber},
			"content":   skydb.FieldType{Type: skydb.TypeString},
		}), ShouldBeNil)

		err := db.Save(&record2)
		So(err, ShouldBeNil)
		err = db.Save(&record1)
		So(err, ShouldBeNil)
		err = db.Save(&record3)
		So(err, ShouldBeNil)

		Convey("queries records", func() {
			query := skydb.Query{
				Type: "note",
			}
			records, err := exhaustRows(db.Query(&query))

			So(err, ShouldBeNil)
			So(records[0], ShouldResemble, record2)
			So(records[1], ShouldResemble, record1)
			So(records[2], ShouldResemble, record3)
			So(len(records), ShouldEqual, 3)
		})

		Convey("sorts queried records ascendingly", func() {
			query := skydb.Query{
				Type: "note",
				Sorts: []skydb.Sort{
					skydb.Sort{
						KeyPath: "noteOrder",
						Order:   skydb.Ascending,
					},
				},
			}
			records, err := exhaustRows(db.Query(&query))

			So(err, ShouldBeNil)
			So(records, ShouldResemble, []skydb.Record{
				record1,
				record2,
				record3,
			})
		})

		Convey("sorts queried records descendingly", func() {
			query := skydb.Query{
				Type: "note",
				Sorts: []skydb.Sort{
					skydb.Sort{
						KeyPath: "noteOrder",
						Order:   skydb.Descending,
					},
				},
			}
			records, err := exhaustRows(db.Query(&query))

			So(err, ShouldBeNil)
			So(records, ShouldResemble, []skydb.Record{
				record3,
				record2,
				record1,
			})
		})

		Convey("query records by note order", func() {
			query := skydb.Query{
				Type: "note",
				Predicate: &skydb.Predicate{
					Operator: skydb.Equal,
					Children: []interface{}{
						skydb.Expression{
							Type:  skydb.KeyPath,
							Value: "noteOrder",
						},
						skydb.Expression{
							Type:  skydb.Literal,
							Value: 1,
						},
					},
				},
			}
			records, err := exhaustRows(db.Query(&query))

			So(err, ShouldBeNil)
			So(records[0], ShouldResemble, record1)
			So(len(records), ShouldEqual, 1)
		})

		Convey("query records by content matching", func() {
			query := skydb.Query{
				Type: "note",
				Predicate: &skydb.Predicate{
					Operator: skydb.Like,
					Children: []interface{}{
						skydb.Expression{
							Type:  skydb.KeyPath,
							Value: "content",
						},
						skydb.Expression{
							Type:  skydb.Literal,
							Value: "Hello%",
						},
					},
				},
			}
			records, err := exhaustRows(db.Query(&query))

			So(err, ShouldBeNil)
			So(records[0], ShouldResemble, record1)
			So(len(records), ShouldEqual, 1)
		})

		Convey("query records by case insensitive content matching", func() {
			query := skydb.Query{
				Type: "note",
				Predicate: &skydb.Predicate{
					Operator: skydb.ILike,
					Children: []interface{}{
						skydb.Expression{
							Type:  skydb.KeyPath,
							Value: "content",
						},
						skydb.Expression{
							Type:  skydb.Literal,
							Value: "hello%",
						},
					},
				},
			}
			records, err := exhaustRows(db.Query(&query))

			So(err, ShouldBeNil)
			So(records[0], ShouldResemble, record1)
			So(len(records), ShouldEqual, 1)
		})

		Convey("query records by check array members", func() {
			query := skydb.Query{
				Type: "note",
				Predicate: &skydb.Predicate{
					Operator: skydb.In,
					Children: []interface{}{
						skydb.Expression{
							Type:  skydb.KeyPath,
							Value: "content",
						},
						skydb.Expression{
							Type:  skydb.Literal,
							Value: []interface{}{"Bye World", "Good Hello", "Anything"},
						},
					},
				},
				Sorts: []skydb.Sort{
					skydb.Sort{
						KeyPath: "noteOrder",
						Order:   skydb.Descending,
					},
				},
			}
			records, err := exhaustRows(db.Query(&query))

			So(err, ShouldBeNil)
			So(records[0], ShouldResemble, record3)
			So(records[1], ShouldResemble, record2)
			So(len(records), ShouldEqual, 2)
		})

		Convey("query records by note order using or predicate", func() {
			keyPathExpr := skydb.Expression{
				Type:  skydb.KeyPath,
				Value: "noteOrder",
			}
			value1 := skydb.Expression{
				Type:  skydb.Literal,
				Value: 2,
			}
			value2 := skydb.Expression{
				Type:  skydb.Literal,
				Value: 3,
			}
			query := skydb.Query{
				Type: "note",
				Predicate: &skydb.Predicate{
					Operator: skydb.Or,
					Children: []interface{}{
						skydb.Predicate{
							Operator: skydb.Equal,
							Children: []interface{}{keyPathExpr, value1},
						},
						skydb.Predicate{
							Operator: skydb.Equal,
							Children: []interface{}{keyPathExpr, value2},
						},
					},
				},
			}
			records, err := exhaustRows(db.Query(&query))

			So(err, ShouldBeNil)
			So(records[0], ShouldResemble, record2)
			So(records[1], ShouldResemble, record3)
			So(len(records), ShouldEqual, 2)
		})

		Convey("query records by offset and paging", func() {
			query := skydb.Query{
				Type:   "note",
				Limit:  2,
				Offset: 1,
				Sorts: []skydb.Sort{
					skydb.Sort{
						KeyPath: "noteOrder",
						Order:   skydb.Descending,
					},
				},
			}
			records, err := exhaustRows(db.Query(&query))

			So(err, ShouldBeNil)
			So(records[0], ShouldResemble, record2)
			So(records[1], ShouldResemble, record1)
			So(len(records), ShouldEqual, 2)
		})
	})

	Convey("Database with reference", t, func() {
		c := getTestConn(t)
		defer cleanupDB(t, c.Db)

		// fixture
		record1 := skydb.Record{
			ID:      skydb.NewRecordID("note", "id1"),
			OwnerID: "user_id",
			Data: map[string]interface{}{
				"noteOrder": float64(1),
			},
		}
		record2 := skydb.Record{
			ID:      skydb.NewRecordID("note", "id2"),
			OwnerID: "user_id",
			Data: map[string]interface{}{
				"noteOrder": float64(2),
				"category":  skydb.NewReference("category", "important"),
			},
		}
		record3 := skydb.Record{
			ID:      skydb.NewRecordID("note", "id3"),
			OwnerID: "user_id",
			Data: map[string]interface{}{
				"noteOrder": float64(3),
				"category":  skydb.NewReference("category", "funny"),
			},
		}
		category1 := skydb.Record{
			ID:      skydb.NewRecordID("category", "important"),
			OwnerID: "user_id",
			Data:    map[string]interface{}{},
		}
		category2 := skydb.Record{
			ID:      skydb.NewRecordID("category", "funny"),
			OwnerID: "user_id",
			Data:    map[string]interface{}{},
		}

		db := c.PrivateDB("userid")
		So(db.Extend("category", skydb.RecordSchema{}), ShouldBeNil)
		So(db.Extend("note", skydb.RecordSchema{
			"noteOrder": skydb.FieldType{Type: skydb.TypeNumber},
			"category": skydb.FieldType{
				Type:          skydb.TypeReference,
				ReferenceType: "category",
			},
		}), ShouldBeNil)

		err := db.Save(&category1)
		So(err, ShouldBeNil)
		err = db.Save(&category2)
		So(err, ShouldBeNil)
		err = db.Save(&record2)
		So(err, ShouldBeNil)
		err = db.Save(&record1)
		So(err, ShouldBeNil)
		err = db.Save(&record3)
		So(err, ShouldBeNil)

		Convey("query records by reference", func() {
			query := skydb.Query{
				Type: "note",
				Predicate: &skydb.Predicate{
					Operator: skydb.Equal,
					Children: []interface{}{
						skydb.Expression{
							Type:  skydb.KeyPath,
							Value: "category",
						},
						skydb.Expression{
							Type:  skydb.Literal,
							Value: skydb.NewReference("category", "important"),
						},
					},
				},
			}
			records, err := exhaustRows(db.Query(&query))

			So(err, ShouldBeNil)
			So(records[0], ShouldResemble, record2)
			So(len(records), ShouldEqual, 1)
		})
	})

	Convey("Database with location", t, func() {
		c := getTestConn(t)
		defer cleanupDB(t, c.Db)

		record0 := skydb.Record{
			ID:      skydb.NewRecordID("restaurant", "0"),
			OwnerID: "someuserid",
			Data: map[string]interface{}{
				"location": skydb.NewLocation(0, 0),
			},
		}
		record1 := skydb.Record{
			ID:      skydb.NewRecordID("restaurant", "1"),
			OwnerID: "someuserid",
			Data: map[string]interface{}{
				"location": skydb.NewLocation(1, 0),
			},
		}
		record2 := skydb.Record{
			ID:      skydb.NewRecordID("restaurant", "2"),
			OwnerID: "someuserid",
			Data: map[string]interface{}{
				"location": skydb.NewLocation(0, 1),
			},
		}

		db := c.PublicDB()
		So(db.Extend("restaurant", skydb.RecordSchema{
			"location": skydb.FieldType{Type: skydb.TypeLocation},
		}), ShouldBeNil)
		So(db.Save(&record0), ShouldBeNil)
		So(db.Save(&record1), ShouldBeNil)
		So(db.Save(&record2), ShouldBeNil)

		Convey("query within distance", func() {
			query := skydb.Query{
				Type: "restaurant",
				Predicate: &skydb.Predicate{
					Operator: skydb.LessThanOrEqual,
					Children: []interface{}{
						skydb.Expression{
							Type: skydb.Function,
							Value: &skydb.DistanceFunc{
								Field:    "location",
								Location: skydb.NewLocation(1, 1),
							},
						},
						skydb.Expression{
							Type:  skydb.Literal,
							Value: 157260,
						},
					},
				},
			}
			records, err := exhaustRows(db.Query(&query))

			So(err, ShouldBeNil)
			So(records, ShouldResemble, []skydb.Record{record0, record1, record2})
		})

		Convey("query within distance with func on R.H.S.", func() {
			query := skydb.Query{
				Type: "restaurant",
				Predicate: &skydb.Predicate{
					Operator: skydb.GreaterThan,
					Children: []interface{}{
						skydb.Expression{
							Type:  skydb.Literal,
							Value: 157260,
						},
						skydb.Expression{
							Type: skydb.Function,
							Value: &skydb.DistanceFunc{
								Field:    "location",
								Location: skydb.NewLocation(1, 1),
							},
						},
					},
				},
			}
			records, err := exhaustRows(db.Query(&query))

			So(err, ShouldBeNil)
			So(records, ShouldResemble, []skydb.Record{record0, record1, record2})
		})

		Convey("query with computed distance", func() {
			query := skydb.Query{
				Type: "restaurant",
				ComputedKeys: map[string]skydb.Expression{
					"distance": skydb.Expression{
						Type: skydb.Function,
						Value: &skydb.DistanceFunc{
							Field:    "location",
							Location: skydb.NewLocation(1, 1),
						},
					},
				},
			}
			records, err := exhaustRows(db.Query(&query))

			So(err, ShouldBeNil)
			So(len(records), ShouldEqual, 3)
			So(records[0].Transient["distance"], ShouldAlmostEqual, 111178, 1)
		})

		Convey("query records ordered by distance", func() {
			query := skydb.Query{
				Type: "restaurant",
				Sorts: []skydb.Sort{
					{
						Func: &skydb.DistanceFunc{
							Field:    "location",
							Location: skydb.NewLocation(0, 0),
						},
						Order: skydb.Desc,
					},
				},
			}

			records, err := exhaustRows(db.Query(&query))
			So(err, ShouldBeNil)
			So(records, ShouldResemble, []skydb.Record{record1, record2, record0})
		})
	})

	Convey("Database with multiple fields", t, func() {
		c := getTestConn(t)
		defer cleanupDB(t, c.Db)

		record0 := skydb.Record{
			ID:      skydb.NewRecordID("restaurant", "0"),
			OwnerID: "someuserid",
			Data: map[string]interface{}{
				"cuisine": "american",
				"title":   "American Restaurant",
			},
		}
		record1 := skydb.Record{
			ID:      skydb.NewRecordID("restaurant", "1"),
			OwnerID: "someuserid",
			Data: map[string]interface{}{
				"cuisine": "chinese",
				"title":   "Chinese Restaurant",
			},
		}
		record2 := skydb.Record{
			ID:      skydb.NewRecordID("restaurant", "2"),
			OwnerID: "someuserid",
			Data: map[string]interface{}{
				"cuisine": "italian",
				"title":   "Italian Restaurant",
			},
		}

		recordsInDB := []skydb.Record{record0, record1, record2}

		db := c.PublicDB()
		So(db.Extend("restaurant", skydb.RecordSchema{
			"title":   skydb.FieldType{Type: skydb.TypeString},
			"cuisine": skydb.FieldType{Type: skydb.TypeString},
		}), ShouldBeNil)
		So(db.Save(&record0), ShouldBeNil)
		So(db.Save(&record1), ShouldBeNil)
		So(db.Save(&record2), ShouldBeNil)

		Convey("query with desired keys", func() {
			query := skydb.Query{
				Type:        "restaurant",
				DesiredKeys: []string{"cuisine"},
			}
			records, err := exhaustRows(db.Query(&query))

			So(err, ShouldBeNil)
			So(len(records), ShouldEqual, 3)
			for i, record := range records {
				So(record.Data["title"], ShouldBeNil)
				So(record.Data["cuisine"], ShouldEqual, recordsInDB[i].Data["cuisine"])
			}
		})

		Convey("query with empty desired keys", func() {
			query := skydb.Query{
				Type:        "restaurant",
				DesiredKeys: []string{},
			}
			records, err := exhaustRows(db.Query(&query))

			So(err, ShouldBeNil)
			So(len(records), ShouldEqual, 3)
			for _, record := range records {
				So(record.Data["title"], ShouldBeNil)
				So(record.Data["cuisine"], ShouldBeNil)
			}
		})

		Convey("query with nil desired keys", func() {
			query := skydb.Query{
				Type:        "restaurant",
				DesiredKeys: nil,
			}
			records, err := exhaustRows(db.Query(&query))

			So(err, ShouldBeNil)
			So(len(records), ShouldEqual, 3)
			for i, record := range records {
				So(record.Data["title"], ShouldEqual, recordsInDB[i].Data["title"])
				So(record.Data["cuisine"], ShouldEqual, recordsInDB[i].Data["cuisine"])
			}
		})

		Convey("query with non-recognized desired keys", func() {
			query := skydb.Query{
				Type:        "restaurant",
				DesiredKeys: []string{"pricing"},
			}
			_, err := exhaustRows(db.Query(&query))

			So(err, ShouldNotBeNil)
		})
	})

	Convey("Empty Conn", t, func() {
		c := getTestConn(t)
		defer cleanupDB(t, c.Db)

		Convey("gets no users", func() {
			userinfo := skydb.UserInfo{}
			err := c.GetUser("notexistuserid", &userinfo)
			So(err, ShouldEqual, skydb.ErrUserNotFound)
		})

		Convey("gets no users with principal", func() {
			userinfo := skydb.UserInfo{}
			err := c.GetUserByPrincipalID("com.example:johndoe", &userinfo)
			So(err, ShouldEqual, skydb.ErrUserNotFound)
		})

		Convey("query no users", func() {
			emails := []string{"user@example.com"}
			result, err := c.QueryUser(emails)
			So(err, ShouldBeNil)
			So(len(result), ShouldEqual, 0)
		})

		Convey("updates no users", func() {
			userinfo := skydb.UserInfo{
				ID: "notexistuserid",
			}
			err := c.UpdateUser(&userinfo)
			So(err, ShouldEqual, skydb.ErrUserNotFound)
		})

		Convey("deletes no users", func() {
			err := c.DeleteUser("notexistuserid")
			So(err, ShouldEqual, skydb.ErrUserNotFound)
		})

		Convey("gets no devices", func() {
			device := skydb.Device{}
			err := c.GetDevice("notexistdeviceid", &device)
			So(err, ShouldEqual, skydb.ErrDeviceNotFound)
		})

		Convey("deletes no devices", func() {
			err := c.DeleteDevice("notexistdeviceid")
			So(err, ShouldEqual, skydb.ErrDeviceNotFound)
		})

		Convey("Empty Database", func() {
			db := c.PublicDB()

			Convey("gets nothing", func() {
				record := skydb.Record{}

				err := db.Get(skydb.NewRecordID("type", "notexistid"), &record)

				So(err, ShouldEqual, skydb.ErrRecordNotFound)
			})

			Convey("deletes nothing", func() {
				err := db.Delete(skydb.NewRecordID("type", "notexistid"))
				So(err, ShouldEqual, skydb.ErrRecordNotFound)
			})

			Convey("queries nothing", func() {
				query := skydb.Query{
					Type: "notexisttype",
				}

				records, err := exhaustRows(db.Query(&query))

				So(err, ShouldBeNil)
				So(records, ShouldBeEmpty)
			})
		})
	})
}

func TestMetaDataQuery(t *testing.T) {
	Convey("Database", t, func() {
		c := getTestConn(t)
		defer cleanupDB(t, c.Db)

		record0 := skydb.Record{
			ID:        skydb.NewRecordID("record", "0"),
			OwnerID:   "ownerID0",
			CreatedAt: time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC),
			CreatorID: "creatorID0",
			UpdatedAt: time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC),
			UpdaterID: "updaterID0",
			Data:      skydb.Data{},
		}
		record1 := skydb.Record{
			ID:        skydb.NewRecordID("record", "1"),
			OwnerID:   "ownerID1",
			CreatedAt: time.Date(2006, 1, 2, 15, 4, 6, 0, time.UTC),
			CreatorID: "creatorID1",
			UpdatedAt: time.Date(2006, 1, 2, 15, 4, 6, 0, time.UTC),
			UpdaterID: "updaterID1",
			Data:      skydb.Data{},
		}

		db := c.PublicDB()
		So(db.Extend("record", nil), ShouldBeNil)
		So(db.Save(&record0), ShouldBeNil)
		So(db.Save(&record1), ShouldBeNil)

		Convey("queries by record id", func() {
			query := skydb.Query{
				Type: "record",
				Predicate: &skydb.Predicate{
					Operator: skydb.Equal,
					Children: []interface{}{
						skydb.Expression{
							Type:  skydb.KeyPath,
							Value: "_id",
						},
						skydb.Expression{
							Type:  skydb.Literal,
							Value: skydb.NewReference("record", "0"),
						},
					},
				},
			}
			records, err := exhaustRows(db.Query(&query))

			So(err, ShouldBeNil)
			So(records, ShouldResemble, []skydb.Record{record0})
		})

		Convey("queries by owner id", func() {
			query := skydb.Query{
				Type: "record",
				Predicate: &skydb.Predicate{
					Operator: skydb.Equal,
					Children: []interface{}{
						skydb.Expression{
							Type:  skydb.KeyPath,
							Value: "_owner_id",
						},
						skydb.Expression{
							Type:  skydb.Literal,
							Value: skydb.NewReference("_user", "ownerID1"),
						},
					},
				},
			}
			records, err := exhaustRows(db.Query(&query))

			So(err, ShouldBeNil)
			So(records, ShouldResemble, []skydb.Record{record1})
		})

		Convey("queries by created at", func() {
			query := skydb.Query{
				Type: "record",
				Predicate: &skydb.Predicate{
					Operator: skydb.LessThan,
					Children: []interface{}{
						skydb.Expression{
							Type:  skydb.KeyPath,
							Value: "_created_at",
						},
						skydb.Expression{
							Type:  skydb.Literal,
							Value: time.Date(2006, 1, 2, 15, 4, 6, 0, time.UTC),
						},
					},
				},
			}
			records, err := exhaustRows(db.Query(&query))

			So(err, ShouldBeNil)
			So(records, ShouldResemble, []skydb.Record{record0})
		})

		Convey("queries by created by", func() {
			query := skydb.Query{
				Type: "record",
				Predicate: &skydb.Predicate{
					Operator: skydb.Equal,
					Children: []interface{}{
						skydb.Expression{
							Type:  skydb.KeyPath,
							Value: "_created_by",
						},
						skydb.Expression{
							Type:  skydb.Literal,
							Value: skydb.NewReference("_user", "creatorID0"),
						},
					},
				},
			}
			records, err := exhaustRows(db.Query(&query))

			So(err, ShouldBeNil)
			So(records, ShouldResemble, []skydb.Record{record0})
		})

		Convey("queries by updated at", func() {
			query := skydb.Query{
				Type: "record",
				Predicate: &skydb.Predicate{
					Operator: skydb.GreaterThan,
					Children: []interface{}{
						skydb.Expression{
							Type:  skydb.KeyPath,
							Value: "_updated_at",
						},
						skydb.Expression{
							Type:  skydb.Literal,
							Value: time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC),
						},
					},
				},
			}
			records, err := exhaustRows(db.Query(&query))

			So(err, ShouldBeNil)
			So(records, ShouldResemble, []skydb.Record{record1})
		})

		Convey("queries by updated by", func() {
			query := skydb.Query{
				Type: "record",
				Predicate: &skydb.Predicate{
					Operator: skydb.Equal,
					Children: []interface{}{
						skydb.Expression{
							Type:  skydb.KeyPath,
							Value: "_updated_by",
						},
						skydb.Expression{
							Type:  skydb.Literal,
							Value: skydb.NewReference("_user", "updaterID1"),
						},
					},
				},
			}
			records, err := exhaustRows(db.Query(&query))

			So(err, ShouldBeNil)
			So(records, ShouldResemble, []skydb.Record{record1})
		})
	})
}