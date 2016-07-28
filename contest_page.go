package main

import "net/http"
import "github.com/naoina/genmai"
import "time"
import "text/template"
import "strconv"
import "fmt"
import "errors"
import "net/url"
import "strings"

var Location = time.FixedZone("Asia/Tokyo", 9 * 60 * 60)

const ContentsPerPage = 50

type ContestsTopHandler struct {
    Temp *template.Template
    NewContest *template.Template
    EachHandler *ContestEachHandler
}

func CreateContestsTopHandler() (*ContestsTopHandler, error) {
    funcs := template.FuncMap{
        "add": func(x, y int) int {return x + y},
        "timeRangeToString": TimeRangeToString,
        "contestTypeToString": func(t int64) string {
            return ContestTypeToString[ContestType(t)]
        },
    }

    temp, err := template.New("").Funcs(funcs).ParseFiles("./html/contests/index_tmpl.html")
    
    if err != nil {
        return nil, err
    }

    temp = temp.Lookup("index_tmpl.html")

    if temp == nil {
        return nil, errors.New("Failed to load /contests/index_temp.html")
    }

    newContest, err := template.ParseFiles("./html/contests/contest_new_tmpl.html")

    if err != nil {
        return nil, err
    }

    ceh, err := CreateContestEachHandler()

    if err != nil {
        return nil, err
    }

    return &ContestsTopHandler{temp, newContest, ceh}, nil
}

func TimeRangeToString(start, finish int64) string {
    startTime := time.Unix(start, 0)
    finishTime := time.Unix(finish, 0)

    return startTime.Format("2006/01/02 15:04:05") + "-" + finishTime.Format("2006/01/02 15:04:05")
}

func (ch ContestsTopHandler) newContestHandler(rw http.ResponseWriter, req *http.Request, std *SessionTemplateData) {
    type TemplateVal struct {
            UserName string
            Msg *string
            StartDate string
            StartTime string
            FinishDate string
            FinishTime string
            Description string
            ContestName string
    }

    if req.Method == "POST" {
        wrapFormStr := func(str string) string {
			arr, has := req.Form[str]
			if has && len(arr) != 0 {
				return arr[0]
			}
			return ""
		}
        startDate, startTime := wrapFormStr("start_date"), wrapFormStr("start_time")
        finishDate, finishTime := wrapFormStr("finish_date"), wrapFormStr("finish_time")
        description := wrapFormStr("description")
        contestName := wrapFormStr("contest_name")

        startStr := startDate + " " + startTime
        finishStr := finishDate + " " + finishTime

        if len(contestName) == 0 {
            msg := "コンテスト名が不正です。"
            templateVal := TemplateVal{
                std.UserID, &msg, startDate, startTime, finishDate, finishTime, description, contestName,
            }

            ch.NewContest.Execute(rw, templateVal)
            
            return
        }
        
        start, err := time.ParseInLocation("2006/01/02 15:04", startStr, Location)

        if err != nil {
            msg := "開始日時の値が不正です。"
            templateVal := TemplateVal{
                std.UserID, &msg, startDate, startTime, finishDate, finishTime, description, contestName,
            }

            ch.NewContest.Execute(rw, templateVal)
            
            return
        }

        finish, err := time.ParseInLocation("2006/01/02 15:04", finishStr, Location)

        if err != nil {
            msg := "終了日時の値が不正です。"
            templateVal := TemplateVal{
                std.UserID, &msg, startDate, startTime, finishDate, finishTime, description, contestName,
            }

            ch.NewContest.Execute(rw, templateVal)
            
            return
        }

        if start.Unix() >= finish.Unix() || start.Unix() < time.Now().Unix() {
            msg := "開始日時及び終了日時の値が不正です。"
            templateVal := TemplateVal{
                std.UserID, &msg, startDate, startTime, finishDate, finishTime, description, contestName,
            }

            ch.NewContest.Execute(rw, templateVal)
            
            return
        }

        cid, err := mainDB.ContestNew(contestName, start.Unix(), finish.Unix(), std.Iid, 0)

        if err != nil {
            if strings.Index(err.Error(), "Duplicate") != -1 {
                msg := "すでに存在するコンテスト名です。"
                templateVal := TemplateVal{
                    std.UserID, &msg, startDate, startTime, finishDate, finishTime, description, contestName,
                }

                ch.NewContest.Execute(rw, templateVal)
            
                return
            }else {
                rw.WriteHeader(http.StatusInternalServerError)
                rw.Write([]byte(ISE500))

                return
            }
        }

        err = mainDB.ContestDescriptionUpdate(cid, description)

        if err != nil {
            fmt.Println(err)
        }

        RespondRedirection(rw, "/contests/" + strconv.FormatInt(cid, 10) + "/")
    }else if req.Method == "GET"{
        templateVal := TemplateVal{
            UserName: std.UserID,
        }

        ch.NewContest.Execute(rw, templateVal)
    }else {
        rw.WriteHeader(http.StatusBadRequest)
        rw.Write([]byte(BR400))
    }
}

func (ch ContestsTopHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
    std, err := ParseRequestForSession(req)

	if std == nil || err != nil || !std.IsSignedIn {
		RespondRedirection(rw, "/login?comeback=/contests" + url.QueryEscape(req.URL.Path))

        return
	}

    err = req.ParseForm()

    if err != nil {
        rw.WriteHeader(http.StatusBadRequest)

        rw.Write([]byte(BR400))

        return
    }

    var cond *genmai.Condition
    timeNow := time.Now().Unix()
    var reqType int

    switch req.URL.Path {
        case "/":
            reqType = 0
            cond = mainDB.db.Where("start_time", "<=", timeNow).And(mainDB.db.Where("finish_time", ">", timeNow))
        case "/coming/":
            reqType = 1
            cond = mainDB.db.Where("start_time", ">", timeNow)
        case "/closed/":
            reqType = 2
            cond = mainDB.db.Where("finish_time", "<=", timeNow)
        case "/new":
            ch.newContestHandler(rw, req, std)

            return
        default:
            if len(req.URL.Path) == 0 {
                RespondRedirection(rw, "/contests/")

                return
            }

            idx := strings.Index(req.URL.Path[1:], "/")

            if idx == -1 {
                RespondRedirection(rw, "/contests" + req.URL.Path + "/")

                return
            }

            cidStr := req.URL.Path[1:][:idx]

            cid, err := strconv.ParseInt(cidStr, 10, 64)

            if err != nil {
                rw.WriteHeader(http.StatusNotFound)
                rw.Write([]byte(NF404))

                return
            }

            handler, err := ch.EachHandler.GetHandler(cid, *std)

            if err != nil {
                rw.WriteHeader(http.StatusNotFound)
                rw.Write([]byte(NF404))
                
                return
            }

            http.StripPrefix("/" + cidStr, handler).ServeHTTP(rw, req)
            
            return
    }

    page := 1

    if queryArr, has := req.Form["p"]; has {
        p, err := strconv.ParseInt(queryArr[0], 10, 64)

        if err != nil || p <= 0 {
            page = 1
        }else {
            page = int(p)
        }
    }

    count, err := mainDB.ContestCount(cond)

    if err != nil {
        //TODO 
        fmt.Println(err) //logに変更
        return
    }

    type TemplateVal struct {
        Contests []Contest
        UserName string
        Type int
        Current int
        MaxPage int
        Pagination []PaginationHelper
    }
    var templateVal TemplateVal
    templateVal.UserName = std.UserName
    templateVal.Type = reqType
    templateVal.Current = 1

    templateVal.MaxPage = int(count) / ContentsPerPage

    if int(count) % ContentsPerPage != 0 {
        templateVal.MaxPage++
    }else if templateVal.MaxPage == 0 {
        templateVal.MaxPage = 1
    }

    if count > 0 {
        if (page - 1) * ContentsPerPage > int(count) {
            page = 1
        }

        templateVal.Current = page

        contests, err := mainDB.ContestList(cond, mainDB.db.OrderBy("start_time", genmai.ASC).Offset(int((page - 1) * ContentsPerPage)).Limit(ContentsPerPage))

        if err == nil {
            templateVal.Contests = *contests
        }else {
            fmt.Println(err)
        }
    }

    templateVal.Pagination = CreatePaginationHelper(templateVal.Current, templateVal.MaxPage, 3)

    rw.WriteHeader(http.StatusOK)
    ch.Temp.Execute(rw, templateVal)

}