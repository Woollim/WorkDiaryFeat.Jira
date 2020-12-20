package main

import (
	"bufio"
	b64 "encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/m7shapan/njson"
)

const (
	confFilePath         = "./configuration.txt"
	csvFilePath          = "./result.csv"
	dateFormat           = "2006-01-02"
	dateFormatForCSV     = "01/02"
	maxTicketCountPerDay = 3
)

func main() {
	conf := getConfiguration()
	if conf != nil {
		fmt.Println("저장되어 있는 Configuration 파일이 있습니다.")
		fmt.Println("기존 데이터를 지우고 싶다면 알파벳\"n\"을 입력해주세요. (그렇지 않다면 아무키나 눌러주세요)")
		var input string
		fmt.Scanln(&input)
		if input == "n" || input == "N" || input == "ㅜ" {
			conf = nil
		}
	}
	if conf == nil {
		var n, bd, sd, pn, wp, cn, ceo, r, url, uid, pd, wst, wet string
		fmt.Print("이름을 입력해주세요: ")
		fmt.Scanln(&n)
		fmt.Print("생년월일을 입력해주세요: ")
		fmt.Scanln(&bd)
		fmt.Print("병역시작일을 입력해주세요: ")
		fmt.Scanln(&sd)
		fmt.Print("본인의 전화번호를 입력해주세요: ")
		fmt.Scanln(&pn)
		fmt.Print("현재 재택근무중인 근무지를 입력해주세요: ")
		fmt.Scanln(&wp)
		fmt.Print("현재 재직중인 회사명을 입력해주세요: ")
		fmt.Scanln(&cn)
		fmt.Print("회사 대표이름을 입력해주세요: ")
		fmt.Scanln(&ceo)
		fmt.Print("재택근무를 해야하는 이유를 입력해주세요: ")
		fmt.Scanln(&r)
		fmt.Print("Jira Workspace URL을 입력해주세요: ")
		fmt.Scanln(&url)
		fmt.Print("Jira UserID를 입력해주세요: ")
		fmt.Scanln(&uid)
		fmt.Print("Jira 비밀번호를 입력해주세요: ")
		fmt.Scanln(&pd)
		fmt.Printf("근무 시작시간을 입력해주세요: ")
		fmt.Scanln(&wst)
		fmt.Printf("근무 종료시간을 입력해주세요: ")
		fmt.Scanln(&wet)
		conf = &Configuration{n, bd, sd, pn, wp, cn, ceo, r, url, uid, pd, wst, wet}
		storeConfiguration(*conf)
	}
	start, end := getDuration()
	list := getTicketList(*conf, start, end)
	writeCSVFile(*conf, list)
}

type Configuration struct {
	// 이름
	Name string
	// 생년월일
	Birthday string
	// 병역시작일
	StartDate string
	// 전화번호
	PhoneNumber string
	// 근무지
	WorkPlace string
	// 회사명
	CompanyName string
	// 대표명
	CEOName string
	// 재택이유
	Reason string
	// Jira Workspace URL
	JiraURL string
	// Jira UserID
	JiraUserID string
	// Jira Password
	JiraPassword string
	// 근무 시작시간
	WorkStartTime string
	// 근무 종료시간
	WorkEndTime string
}

func getConfiguration() *Configuration {
	contents, err := ioutil.ReadFile(confFilePath)
	if err != nil {
		fmt.Println("저장되어 있는 사용자 정보가 없습니다. Script를 실행하기 위한 사용자 정보를 입력해주세요.")
		return nil
	}
	var conf *Configuration
	err = json.Unmarshal(contents, &conf)
	if err != nil {
		fmt.Println("사용자 정보가 올바르게 가져올 수 없습니다. Script를 실행하기 위한 사용자 정보를 다시 입력해주세요.\n(기존 정보는 삭제됩니다.)")
		return nil
	}
	return conf
}

func storeConfiguration(conf Configuration) {
	json, err := json.Marshal(conf)
	if err != nil {
		println("Unexpected Error", err.Error())
		panic("사용자의 정보를 제대로 저장하지 못했습니다. Script를 종료 후 다시 실행해주세요.")
	}
	err = ioutil.WriteFile(confFilePath, json, 0777)
	if err != nil {
		println("Unexpected lError", err.Error())
		panic("사용자의 정보를 제대로 저장하지 못했습니다. Script를 종료 후 다시 실행해주세요.")
	}
}

func getDuration() (time.Time, time.Time) {
	var startDate, endDate time.Time
	whileWhenNotError(func() error {
		var input string
		var err error
		fmt.Print("시작 날짜를 입력해주세요 (해당 포맷에 맞추어: yyyy-MM-dd): ")
		fmt.Scanln(&input)
		startDate, err = time.Parse(dateFormat, input)
		return err
	})
	whileWhenNotError(func() error {
		var input string
		var err error
		fmt.Print("종료 날짜를 입력해주세요 (해당 포맷에 맞추어: yyyy-MM-dd): ")
		fmt.Scanln(&input)
		endDate, err = time.Parse(dateFormat, input)
		return err
	})
	return startDate, endDate
}

func whileWhenNotError(executor func() error) {
	err := executor()
	for err != nil {
		fmt.Println("올바르지 않은 입력입니다. 다시 시도해주세요.")
		err = executor()
	}
	return
}

type TimeWithJiraTickets struct {
	Time    time.Time
	tickets JiraTickets
}

type JiraTickets struct {
	Tickets []JiraTicket `njson:"issues"`
}

type JiraTicket struct {
	Key            string `njson:"key"`
	Summary        string `njson:"fields.summary"`
	Description    string `njson:"fields.description"`
	CreatedDate    string `njson:"fields.created"`
	UpdatedDate    string `njson:"fields.updated"`
	ResolutionDate string `njson:"fields.resolutiondate"`
	ParentKey      string `njson:"fields.parent.key"`
	ParentSummary  string `njson:"fields.parent.fields.summary"`
}

func getTicketList(conf Configuration, start time.Time, end time.Time) []TimeWithJiraTickets {
	println("HTTP 통신을 사용하여 Jira에 등록되어 있는 Ticket을 가져오고 있습니다.")

	list := make([]TimeWithJiraTickets, 0)
	for t := start; !t.After(end); t = t.Add(time.Hour * 24) {
		tStr := t.Format(dateFormat)
		jql := fmt.Sprintf("assignee=%s and created<=%s and resolved>=%s", conf.JiraUserID, tStr, tStr)
		reqURL := fmt.Sprintf("%s/rest/api/2/search?jql=%s", conf.JiraURL, url.PathEscape(jql))
		req, err := http.NewRequest("GET", reqURL, nil)
		if err != nil {
			fmt.Printf("%s의 Jira Request를 만들지 못했습니다.\n", tStr)
			fmt.Println(err.Error())
			continue
		}
		token := b64.StdEncoding.EncodeToString([]byte(conf.JiraUserID + ":" + conf.JiraPassword))
		req.Header.Add("Authorization", "Basic "+token)
		req.Header.Add("Content-Type", "application/json")
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("%s의 HTTP 통신을 성공하지 못했습니다.\n", tStr)
			fmt.Println(err.Error())
			continue
		}
		bytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			fmt.Printf("%s의 Jira Response을 올바르게 읽지 못했습니다.\n", tStr)
			fmt.Println(err.Error())
			continue
		}
		if resp.StatusCode != 200 {
			fmt.Printf("%s의 Jira Error Response 입니다.\n", tStr)
			println(string(bytes))
		}
		var tickets JiraTickets
		err = njson.Unmarshal(bytes, &tickets)
		if err != nil {
			fmt.Printf("%s의 Jira Response을 올바르게 읽지 못했습니다.\n", tStr)
			fmt.Println(err.Error())
			continue
		}
		list = append(list, TimeWithJiraTickets{t, tickets})
		resp.Body.Close()
	}
	return list
}

func writeCSVFile(conf Configuration, list []TimeWithJiraTickets) {
	println("Jira Ticket을 기반으로 CSV 파일을 작성합니다.")

	if _, err := os.Stat(csvFilePath); err == nil {
		os.Remove(csvFilePath)
	}
	f, err := os.Create(csvFilePath)
	if err != nil {
		fmt.Println("CSV 파일을 생성하지 못했습니다. file path를 확인해주세요.")
		fmt.Printf("현재 경로: %s\n", csvFilePath)
		fmt.Println(err.Error())
		return
	}
	defer f.Close()

	wr := csv.NewWriter(bufio.NewWriter(f))
	// 업무 시작 시간, 업무 종료 시간 컬럼 (2, 3번 컬럼)에 빈 string 입력
	wr.Write([]string{"이름", conf.Name, "", ""})
	wr.Write([]string{"생년월일", conf.Birthday, "", ""})
	wr.Write([]string{"병역시작일", conf.StartDate, "", ""})
	wr.Write([]string{"전화번호", conf.PhoneNumber, "", ""})
	wr.Write([]string{"근무지", conf.WorkPlace, "", ""})
	wr.Write([]string{"회사명", conf.CompanyName, "", ""})
	wr.Write([]string{"대표명", conf.CEOName, "", ""})
	wr.Write([]string{"재택 사유", conf.Reason, "", ""})
	for i := 0; i < len(list); i++ {
		value := list[i]
		currentCount := min(len(value.tickets.Tickets), maxTicketCountPerDay)
		if currentCount == 0 {
			wr.Write([]string{value.Time.Format(dateFormatForCSV), "내용없음", conf.WorkStartTime, conf.WorkEndTime})
			continue
		}
		content := ""
		for j := 0; j < currentCount; j++ {
			content += (value.tickets.Tickets[j].Summary)
			if j < (currentCount - 1) {
				content += "\n"
			}
		}
		wr.Write([]string{value.Time.Format(dateFormatForCSV), content, conf.WorkStartTime, conf.WorkEndTime})
	}
	wr.Flush()
}

func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}
