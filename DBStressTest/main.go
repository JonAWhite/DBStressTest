package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"database/sql"
	_ "odbc/driver"
	"time"
	"runtime"
	"os"

	"github.com/gorilla/mux"
)

// error response contains everything we need to use http.Error
type handlerError struct {
	Error   error
	Message string
	Code    int
}

// stress_test model
type stress_test struct {
	Name   	  string  `json:"name"`
	Parallel  int     `json:"parallel"`
	Id        int     `json:"id"`
	Run       string  `json:"run"`
	Duration  float64 `json:"duration"`
}

// list of all of the stress tests
var stress_tests = make([]stress_test, 0)

// a custom type that we can use for handling errors and formatting responses
type handler func(w http.ResponseWriter, r *http.Request) (interface{}, *handlerError)

// attach the standard ServeHTTP method to our handler so the http library can call it
func (fn handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// here we could do some prep work before calling the handler if we wanted to

	// call the actual handler
	response, err := fn(w, r)

	// check for errors
	if err != nil {
		log.Printf("ERROR: %v\n", err.Error)
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Message), err.Code)
		return
	}
	if response == nil {
		log.Printf("ERROR: response from method is nil\n")
		http.Error(w, "Internal server error. Check the logs.", http.StatusInternalServerError)
		return
	}

	// turn the response into JSON
	bytes, e := json.Marshal(response)
	if e != nil {
		http.Error(w, "Error marshalling JSON", http.StatusInternalServerError)
		return
	}

	// send the response and log
	w.Header().Set("Content-Type", "application/json")
	w.Write(bytes)
	log.Printf("%s %s %s %d", r.RemoteAddr, r.Method, r.URL, 200)
}

func listStressTests(w http.ResponseWriter, r *http.Request) (interface{}, *handlerError) {
	return stress_tests, nil
}

func getStressTest(w http.ResponseWriter, r *http.Request) (interface{}, *handlerError) {
	// mux.Vars grabs variables from the path
	param := mux.Vars(r)["id"]
	id, e := strconv.Atoi(param)
	if e != nil {
		return nil, &handlerError{e, "Id should be an integer", http.StatusBadRequest}
	}
	b, index := getStressTestById(id)

	if index < 0 {
		return nil, &handlerError{nil, "Could not find stress test " + param, http.StatusNotFound}
	}

	return b, nil
}

func parseStressTestRequest(r *http.Request) (stress_test, *handlerError) {
	// the stress_test payload is in the request body
	data, e := ioutil.ReadAll(r.Body)
	if e != nil {
		return stress_test{}, &handlerError{e, "Could not read request", http.StatusBadRequest}
	}

	// turn the request body (JSON) into a stress_test object
	var payload stress_test
	e = json.Unmarshal(data, &payload)
	if e != nil {
		return stress_test{}, &handlerError{e, "Could not parse JSON", http.StatusBadRequest}
	}

	return payload, nil
}

func addStressTest(w http.ResponseWriter, r *http.Request) (interface{}, *handlerError) {
	payload, e := parseStressTestRequest(r)
	if e != nil {
		return nil, e
	}

	// it's our job to assign IDs, ignore what (if anything) the client sent
	payload.Id = getNextId()
	stress_tests = append(stress_tests, payload)

	// we return the stress_test we just made so the client can see the ID if they want
	return payload, nil
}

func updateStressTest(w http.ResponseWriter, r *http.Request) (interface{}, *handlerError) {
	payload, e := parseStressTestRequest(r)
	if e != nil {
		return nil, e
	}

	_, index := getStressTestById(payload.Id)
	stress_tests[index] = payload
	return make(map[string]string), nil
}

func removeStressTest(w http.ResponseWriter, r *http.Request) (interface{}, *handlerError) {
	param := mux.Vars(r)["id"]
	id, e := strconv.Atoi(param)
	if e != nil {
		return nil, &handlerError{e, "Id should be an integer", http.StatusBadRequest}
	}
	// this is jsut to check to see if the stress_test exists
	_, index := getStressTestById(id)

	if index < 0 {
		return nil, &handlerError{nil, "Could not find entry " + param, http.StatusNotFound}
	}

	// remove a stress_test from the list
	stress_tests = append(stress_tests[:index], stress_tests[index+1:]...)
	return make(map[string]string), nil
}

// searches the stress_tests for the stress_test with `id` and returns the stress_test and it's index, or -1 for 404
func getStressTestById(id int) (stress_test, int) {
	for i, s := range stress_tests {
		if s.Id == id {
			return s, i
		}
	}
	return stress_test{}, -1
}

var id = 0

// increments id and returns the value
func getNextId() int {
	id += 1
	return id
}

func runStressTest(w http.ResponseWriter, r *http.Request) (interface{}, *handlerError) {
	// mux.Vars grabs variables from the path
	param := mux.Vars(r)["id"]
	id, _ := strconv.Atoi(param)
	st, index := getStressTestById(id)
	log.Println(st.Parallel)
	//runtime.GOMAXPROCS(st.Parallel)

	//done := make(chan bool)
	t1 := time.Now()
	for i := 0; i < st.Parallel; i++ {
		//go func() {
			db, err := sql.Open("odbc", "DSN=verticaTest;")
			defer db.Close()
			if err != nil {
				log.Println(err)
			}

			stmt, err :=	db.Prepare("select * from test_table")
			defer stmt.Close()
			if err != nil {
				log.Println(err)
			}

			rows, err :=	stmt.Query()
			checkErr(err)
			printTable(rows)
			//done <- true
		//}()
	}

	//for j := 0; j < st.Parallel; j++ {
	//	<-done
	//}

	t2 := time.Now()
	dur := t2.Sub(t1)
	const layout = "Jan 2, 2006 at 3:04:01pm (EST)"
	st.Run = t1.Format(layout)
	st.Duration = dur.Seconds()
	stress_tests[index] = st
	return st,nil
}

func printTable(rows *sql.Rows) { 
/*
    pr := func(t interface{}) (r string) { 
            r = "\\N" 
            switch v := t.(type) { 
            case *sql.NullBool: 
                if v.Valid { 
                    r = fmt.Sprintf("%v", v.Bool) 
                } 
            case *sql.NullString: 
                if v.Valid { 
                    r = v.String 
                } 
            case *sql.NullInt64: 
                if v.Valid { 
                    r = fmt.Sprintf("%6d", v.Int64) 
                } 
            case *sql.NullFloat64: 
                if v.Valid { 
                    r = fmt.Sprintf("%.2f", v.Float64) 
                } 
            case *time.Time: 
                if v.Year() > 1900 { 
                    r = v.Format("_2 Jan 2006") 
                } 
            default: 
                r = fmt.Sprintf("%#v", t) 
            } 
            return 
        } 

        c, _ := rows.Columns() 
        n := len(c) 

        // print labels 
        for i := 0; i < n; i++ { 
            if len(c[i]) > 1 && c[i][1] == ':' { 
                fmt.Print(c[i][2:], "\t") 
            } else { 
                fmt.Print(c[i], "\t") 
            } 
        } 
        fmt.Print("\n\n") 

        // print data 
        var field []interface{} 
        for i := 0; i < n; i++ { 
            switch { 
            case c[i][:2] == "b:": 
                field = append(field, new(sql.NullBool)) 
            case c[i][:2] == "f:": 
                field = append(field, new(sql.NullFloat64)) 
            case c[i][:2] == "i:": 
                field = append(field, new(sql.NullInt64)) 
            case c[i][:2] == "s:": 
                field = append(field, new(sql.NullString)) 
            case c[i][:2] == "t:": 
                field = append(field, new(time.Time)) 
            default: 
                field = append(field, new(sql.NullString)) 
            } 
        } 
        for rows.Next() { 
            checkErr(rows.Scan(field...)) 
            for i := 0; i < n; i++ { 
                fmt.Print(pr(field[i]), "\t") 
            } 
            fmt.Println() 
        } 
        fmt.Println() 
*/} 

func checkErr(err error) { 
    if err != nil { 
        _, filename, lineno, ok := runtime.Caller(1) 
        if ok { 
            fmt.Fprintf(os.Stderr, "%v:%v: %v\n", filename, 
lineno, err) 
        } 
        panic(err) 
    } 
} 


func main() {
	// command line flags
	port := flag.Int("port", 80, "port to serve on")
	dir := flag.String("directory", "web/", "directory of web files")
	flag.Parse()

	// handle all requests by serving a file of the same name
	fs := http.Dir(*dir)
	fileHandler := http.FileServer(fs)

	// setup routes
	router := mux.NewRouter()
	router.Handle("/", http.RedirectHandler("/static/", 302))
	router.Handle("/stress_tests", handler(listStressTests)).Methods("GET")
	router.Handle("/stress_tests", handler(addStressTest)).Methods("POST")
	router.Handle("/stress_tests/{id}", handler(getStressTest)).Methods("GET")
	router.Handle("/stress_tests/{id}", handler(updateStressTest)).Methods("POST")
	router.Handle("/stress_tests/{id}", handler(removeStressTest)).Methods("DELETE")
	router.Handle("/stress_tests/{id}/run", handler(runStressTest)).Methods("POST")
	router.PathPrefix("/static/").Handler(http.StripPrefix("/static", fileHandler))
	http.Handle("/", router)

	// bootstrap some data
	stress_tests = append(stress_tests, stress_test{"Single Fetch", 1, getNextId(), "", 0})
	stress_tests = append(stress_tests, stress_test{"Twenty-Five in Parallel", 25, getNextId(), "", 0})
	stress_tests = append(stress_tests, stress_test{"Fifty in Parallel", 50, getNextId(), "", 0})
	stress_tests = append(stress_tests, stress_test{"Seventy-Five in Parallel", 75, getNextId(), "", 0})

	log.Printf("Running on port %d\n", *port)

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	// this call blocks -- the progam runs here forever
	err := http.ListenAndServe(addr, nil)
	fmt.Println(err.Error())
}
