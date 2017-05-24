// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import _ "github.com/denisenkom/go-mssqldb"
import (
	"database/sql"
	"fmt"
	"github.com/bitly/go-simplejson"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

//define global varibles of server connection info
var (
	staging_server      string
	staging_port        int
	staging_user        string
	staging_password    string
	production_server   string
	production_port     int
	production_user     string
	production_password string

	host_port string
)

type IndexData struct {
	ClientNames []string
}

type ResultData struct {
	ClientName string
	XMLs       []XMLResult
	Rules      []RuleResult
}

type XML struct {
	UID, XMLConfig string
}

type XMLResult struct {
	XMLName       string
	XMLStaging    string
	XMLProduction string
}

type SorterXML []XML

func (sorter SorterXML) Len() int {
	return len(sorter)
}
func (sorter SorterXML) Less(i, j int) bool {
	return sorter[i].UID < sorter[j].UID
}
func (sorter SorterXML) Swap(i, j int) {
	sorter[i], sorter[j] = sorter[j], sorter[i]
}

type Rule struct {
	RuleID, RuleType, RuleText string
}

type RuleResult struct {
	RuleID         string
	RuleType       string
	RuleStaging    string
	RuleProduction string
}

type SorterRule []Rule

func (sorter SorterRule) Len() int {
	return len(sorter)
}
func (sorter SorterRule) Less(i, j int) bool {
	return sorter[i].RuleID < sorter[j].RuleID
}
func (sorter SorterRule) Swap(i, j int) {
	sorter[i], sorter[j] = sorter[j], sorter[i]
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	datebases := retrievedblist()
	database := IndexData{
		ClientNames: datebases}

	err := templates.ExecuteTemplate(w, "index.html", database)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func compareHandler(w http.ResponseWriter, r *http.Request) {
	clientname := r.FormValue("selClientName")
	// fmt.Println(clientname)

	resultdata := ResultData{
		ClientName: clientname,
		XMLs:       comparexml(clientname),
		Rules:      comparerule(clientname)}

	// http.Redirect(w, r, "/index/", http.StatusFound)

	err := templates.ExecuteTemplate(w, "result.html", resultdata)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

var templates *template.Template

// add index
// var validPath = regexp.MustCompile("^/(edit|save|view|index)/([a-zA-Z0-9]+)$")

var validPath = regexp.MustCompile("^/(index|compare|new)/")

func makeHandler(fn func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m := validPath.FindStringSubmatch(r.URL.Path)
		if m == nil {
			http.NotFound(w, r)
			return
		}
		// fmt.Println(m)
		// fn(w, r, m[2])
		fn(w, r)
	}
}

func sqldbconnection(server string, database string, port int, user string, password string) *sql.DB {
	connString := fmt.Sprintf("server=%s;user id=%s;password=%s;port=%d;database=%s", server, user, password, port, database)
	conn, err := sql.Open("mssql", connString)
	if err != nil {
		log.Printf("Open connection failed:", err.Error())
		conn = nil
	}
	return conn
}

func retrievedblist() []string {
	conn := sqldbconnection(staging_server, "master", staging_port, staging_user, staging_password)
	defer conn.Close()

	DBliststatement := "SELECT name FROM master..sysdatabases WHERE (name like 'BA2_%%_Staging' OR name like 'BA2_%%_Staging_new') and name not like 'BA2_CT_%%' and name not like '%Portal%';"

	stmt, err := conn.Prepare(DBliststatement)
	if err != nil {
		log.Fatal("Prepare failed:", err.Error())
	}
	defer stmt.Close()

	row, err := stmt.Query()
	if err != nil {
		fmt.Println("Query Error", err)
		panic(err)
	}
	defer row.Close()

	var databases []string
	var database string

	for row.Next() {
		err := row.Scan(&database)

		if err == nil {
			database = strings.TrimPrefix(database, "BA2_")
			database = strings.TrimSuffix(database, "_Staging")
			database = strings.TrimSuffix(database, "_STAGING")
			databases = append(databases, database)
		}
	}
	return databases
}

func retrievexml(conn *sql.DB) SorterXML {
	stmt, err := conn.Prepare("select XMLConfiguration.UID, CONVERT(nvarchar(max), XMLConfiguration.XMLConfig) from dbo.XMLConfiguration order by xmlconfiguration.UID;")

	if err != nil {
		log.Printf("Prepare failed:", err.Error())
		return nil
	}

	defer stmt.Close()

	row, err := stmt.Query()
	if err != nil {
		fmt.Println("Query Error", err)
		return nil
	}
	defer row.Close()

	var xmls SorterXML
	var uid, xmlconfig string

	for row.Next() {
		// var onexml XML
		// err := row.Scan(&onexml.UID, &onexml.XMLConfig)
		err := row.Scan(&uid, &xmlconfig)

		if err == nil {
			xmls = append(xmls, XML{uid, xmlconfig})
		}
	}
	return xmls
}

func comparexml(clientname string) []XMLResult {
	//retrieve xml from staging
	connstaging := sqldbconnection(staging_server, "BA2_"+clientname+"_Staging", staging_port, staging_user, staging_password)
	defer connstaging.Close()
	xmlliststaging := retrievexml(connstaging)
	sort.Sort(xmlliststaging)
	// fmt.Println("done staging")

	//retrieve xml from production
	connproduction := sqldbconnection(production_server, "BA2_"+clientname, production_port, production_user, production_password)
	defer connproduction.Close()
	xmllistproduction := retrievexml(connproduction)
	sort.Sort(xmllistproduction)
	// fmt.Println("done production")

	//compare
	var resultlist []XMLResult
	s := 0
	p := 0
	for s < len(xmlliststaging) && p < len(xmllistproduction) {
		if xmlliststaging[s].UID == xmllistproduction[p].UID {
			//compare xml
			if !reflect.DeepEqual(xmlliststaging[s].XMLConfig, xmllistproduction[p].XMLConfig) {
				resultlist = append(resultlist, XMLResult{XMLName: xmlliststaging[s].UID, XMLStaging: xmlliststaging[s].XMLConfig, XMLProduction: xmllistproduction[p].XMLConfig})
			}
			s++
			p++
		} else if xmlliststaging[s].UID < xmllistproduction[p].UID || p >= len(xmllistproduction) {
			// staging only
			resultlist = append(resultlist, XMLResult{XMLName: xmlliststaging[s].UID, XMLStaging: xmlliststaging[s].XMLConfig})
			s++
		} else if xmlliststaging[s].UID > xmllistproduction[p].UID || s >= len(xmlliststaging) {
			// production only
			resultlist = append(resultlist, XMLResult{XMLName: xmllistproduction[p].UID, XMLProduction: xmllistproduction[p].XMLConfig})
			p++
		}
	}

	// for _, result := range resultlist {
	// 	fmt.Println(result.XMLName)
	// }
	return resultlist
}

func retrieverule(conn *sql.DB) SorterRule {
	stmt, err := conn.Prepare("select rulelibrary.ruleid, rulelibrary.ruletype, convert(nvarchar(max), rulelibrary.ruletext) from dbo.rulelibrary order by rulelibrary.ruleid;")

	if err != nil {
		log.Printf("Prepare rule selection failed:", err.Error())
		return nil
	}

	defer stmt.Close()

	row, err := stmt.Query()
	if err != nil {
		fmt.Println("Query Error", err)
		return nil
	}
	defer row.Close()

	var rules SorterRule
	var ruleID, ruletype, ruletext string

	for row.Next() {
		err := row.Scan(&ruleID, &ruletype, &ruletext)

		if err == nil {
			rules = append(rules, Rule{ruleID, ruletype, ruletext})
		}
	}
	return rules
}

func comparerule(clientname string) []RuleResult {
	//retrieve rule from staging
	connstaging := sqldbconnection(staging_server, "BA2_"+clientname+"_Staging", staging_port, staging_user, staging_password)
	defer connstaging.Close()
	ruleliststaging := retrieverule(connstaging)
	sort.Sort(ruleliststaging)

	connproduction := sqldbconnection(production_server, "BA2_"+clientname, production_port, production_user, production_password)
	defer connproduction.Close()
	rulelistproduction := retrieverule(connproduction)
	sort.Sort(rulelistproduction)

	//compare
	var resultlist []RuleResult
	s := 0
	p := 0
	for s < len(ruleliststaging) && p < len(rulelistproduction) {
		if ruleliststaging[s].RuleID == rulelistproduction[p].RuleID {
			//compare rule
			if !reflect.DeepEqual(ruleliststaging[s].RuleText, rulelistproduction[p].RuleText) {
				resultlist = append(resultlist, RuleResult{RuleID: ruleliststaging[s].RuleID, RuleType: ruleliststaging[s].RuleType, RuleStaging: ruleliststaging[s].RuleText, RuleProduction: rulelistproduction[p].RuleText})
			}
			s++
			p++
		} else if ruleliststaging[s].RuleID < rulelistproduction[p].RuleID || p >= len(rulelistproduction) {
			// staging only
			resultlist = append(resultlist, RuleResult{RuleID: ruleliststaging[s].RuleID, RuleType: ruleliststaging[s].RuleType, RuleStaging: ruleliststaging[s].RuleText})
			s++
		} else if ruleliststaging[s].RuleID > rulelistproduction[p].RuleID || s >= len(ruleliststaging) {
			// production only
			resultlist = append(resultlist, RuleResult{RuleID: rulelistproduction[p].RuleID, RuleType: ruleliststaging[s].RuleType, RuleProduction: rulelistproduction[p].RuleText})
			p++
		}
	}
	return resultlist
}

func ReadConfiguration() {
	configfile, err := ioutil.ReadFile("config.json")
	if err != nil {
		fmt.Println("configuration file cannot be accessed")
		panic(err)
	}

	js, err := simplejson.NewJson(configfile)
	if err != nil {
		fmt.Println("configuration file content seems wrong")
		panic(err)
	}

	staging_server, err = js.Get("staging_server").String()
	staging_port, err = js.Get("staging_port").Int()
	staging_user, err = js.Get("staging_user").String()
	staging_password, err = js.Get("staging_password").String()
	production_server, err = js.Get("production_server").String()
	production_port, err = js.Get("production_port").Int()
	production_user, err = js.Get("production_user").String()
	production_password, err = js.Get("production_password").String()

	host_port, err = js.Get("host_port").String()
}

func main() {
	templates = template.Must(template.ParseFiles(filepath.Join("template", "index.html"), filepath.Join("template", "result.html")))

	//get sql servers' configuration
	ReadConfiguration()

	http.Handle("/resources/", http.StripPrefix("/resources/", http.FileServer(http.Dir("resources"))))

	//add index handler
	http.HandleFunc("/index/", makeHandler(indexHandler))
	http.HandleFunc("/compare/", makeHandler(compareHandler))

	fmt.Println("start...")

	http.ListenAndServe(host_port, nil)
}
