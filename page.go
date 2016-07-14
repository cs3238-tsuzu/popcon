package main

import (
	"crypto/sha512"
	"errors"
	"fmt"
	"net/http"
	"os"
	"text/template"
	"reflect"
)

// PageHandlerFuncType is a type of the function used in PageHandler
type PageHandlerFuncType func(http.ResponseWriter, *http.Request)

// PageHandler a handler for each page
type PageHandler struct {
	Callback PageHandlerFuncType
}

// PassHandler is a function that pass a handler
func (ph *PageHandler) PassHandler() func(http.ResponseWriter, *http.Request) {
	return func(rw http.ResponseWriter, req *http.Request) {
		ph.Callback(rw, req)
	}
}

// FuncHandlerWrapepr is for FuncToHandler
type FuncHandlerWrapepr struct {
	f func(http.ResponseWriter, *http.Request)
}

func (fhw FuncHandlerWrapepr) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	fhw.f(rw, req)
}

// FuncToHandler wraps functions as a handler
func FuncToHandler(f func(http.ResponseWriter, *http.Request)) FuncHandlerWrapepr {
	return FuncHandlerWrapepr{f}
}

// CreateHandlers is a function to return hadlers
func CreateHandlers() (*map[string]*PageHandler, error) {
	res := make(map[string]*PageHandler)

	var err error

	res["/"], err = func() (*PageHandler, error) {
		tmp, err := template.ParseFiles("./html/index_tmpl.html")

		if err != nil {
			return nil, errors.New("Failed to load ./html/index_tmpl.html")
		}

		f := func(rw http.ResponseWriter, req *http.Request) {
			if req.URL.Path != "/" && req.URL.Path != "/#" {
				rw.WriteHeader(http.StatusNotFound)

				fmt.Fprint(rw, NF404)

				return
			}

			std, err := ParseRequestForSession(req)

			if std == nil || err != nil {
				std = &SessionTemplateData{
					IsSignedIn: false,
					UserID:     "",
					UserName:   "",
				}
			}

			news, err := mainDB.NewsGet()

			if err != nil {
				news = make([]News, 0)

				fmt.Println(err.Error())
			}

			type IndexResp struct {
				*SessionTemplateData
				News      []News
				NewsCount int
			}

			resp := &IndexResp{std, news, mainDB.showedNewCount}

			rw.WriteHeader(http.StatusOK)
			tmp.Execute(rw, *resp)
		}

		return &PageHandler{f}, nil
	}()

	if err != nil {
		return nil, err
	}

	res["/onlinejudge/"], err = func() (*PageHandler, error) {
		ojh := http.StripPrefix("/onlinejudge/", CreateOnlineJudgeHandler())

		if ojh != nil {
			return nil, errors.New("Failed to CreateOnlineJudgeHandler()")
		}

		f := func(rw http.ResponseWriter, req *http.Request) {
			ojh.ServeHTTP(rw, req)
		}

		return &PageHandler{f}, nil
	}()

	res["/contests/"], err = func() (*PageHandler, error) {
		f := func(rw http.ResponseWriter, req *http.Request) {
			rw.WriteHeader(http.StatusNotImplemented)

			fmt.Fprint(rw, NI501)
		}

		return &PageHandler{f}, nil
	}()

	res["/login"], err = func() (*PageHandler, error) {
		type LoginTemp struct {
			IsFailed bool
			BackURL  string
		}

		tmp, err := template.ParseFiles("./html/login_tmpl.html")

		if err != nil {
			return nil, errors.New("Failed to load ./html/login_tmpl.html")
		}

		f := func(rw http.ResponseWriter, req *http.Request) {
			if req.Method == "GET" {
				req.ParseForm()
				rw.WriteHeader(http.StatusOK)

				comeback, res := req.Form["comeback"]

				var cburl string
				if !res || len(comeback) == 0 || len(comeback[0]) == 0 {
					cburl = "/"
				} else {
					cburl = comeback[0]
				}

				tmp.Execute(rw, LoginTemp{false, cburl})
			} else if req.Method == "POST" {
				if req.ParseForm() != nil {
					rw.WriteHeader(http.StatusBadRequest)
					rw.Write(nil)

					return
				}

				loginID, res := req.Form["loginID"]
				password, res2 := req.Form["password"]
				backurl, res3 := req.Form["comeback"]

				if !res || !res2 || !res3 || len(loginID) == 0 || len(password) == 0 || len(backurl) == 0 || len(loginID) > 20 || len(backurl[0]) == 0 {
					rw.WriteHeader(http.StatusBadRequest)
					fmt.Fprint(rw, BR400)

					return
				}

				if backurl[0][0] != '/' {
					rw.WriteHeader(http.StatusBadRequest)
					fmt.Fprint(rw, BR400)

					return
				}

				user, err := mainDB.UserFindFromUserID(loginID[0])
				passHash := sha512.Sum512([]byte(password[0]))

				if err != nil || !reflect.DeepEqual(user.PassHash, passHash[:]) {
					rw.WriteHeader(http.StatusOK)

					tmp.Execute(rw, LoginTemp{true, backurl[0]})

					return
				}

				sessionID, err := mainDB.SessionAdd(user.Iid)

				if err != nil {
					rw.WriteHeader(http.StatusInternalServerError)

					fmt.Fprint(rw, ISE500)
				} else {
					cookie := http.Cookie{
						Name:   "session",
						Value:  *sessionID,
						MaxAge: 2592000,
					}

					http.SetCookie(rw, &cookie)
					RespondRedirection(rw, backurl[0])

					rw.Write(nil)
				}
			} else {
				rw.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(rw, BR400)
			}

		}

		return &PageHandler{f}, nil
	}()

	res["/logout"], err = func() (*PageHandler, error) {
		f := func(rw http.ResponseWriter, req *http.Request) {
			session := ParseSession(req)

			if session != nil {
				mainDB.SessionRemove(*session)
			}

			cookie := http.Cookie{
				Name:   "session",
				Value:  *session,
				MaxAge: 0,
			}

			http.SetCookie(rw, &cookie)
			RespondRedirection(rw, "/")
		}

		return &PageHandler{f}, nil
	}()

	res["/userinfo"], err = func() (*PageHandler, error) {
		tmp, err := template.ParseFiles("./html/userinfo_tmpl.html")

		if err != nil {
			return nil, err
		}

		f := func(rw http.ResponseWriter, req *http.Request) {
			user, err := ParseRequestForUseData(req)

			if err != nil {
				RespondRedirection(rw, "/login?comeback=/userinfo")

				return
			}

			rw.WriteHeader(http.StatusOK)
			tmp.Execute(rw, user)
		}

		return &PageHandler{f}, nil
	}()

	// Debug request
	res["/quit"] = &PageHandler{
		func(_ http.ResponseWriter, _ *http.Request) {
			os.Exit(0)
		}}

	if err != nil {
		return nil, err
	}

	return &res, nil
}