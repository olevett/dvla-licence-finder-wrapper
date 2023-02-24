package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/antchfx/htmlquery"
	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

type Gender int

const (
	Unknown Gender = 0
	Male    Gender = 1
	Female  Gender = 2
)

type DrivingLicenceRequest struct {
	Nino        string    `json:"nino"`
	Forename    string    `json:"forename"`
	Surname     string    `json:"surname"`
	Postcode    string    `json:"postcode"`
	Gender      Gender    `json:"gender"`
	DateOfBirth time.Time `json:"dateOfBirth"`
}

func (dlr *DrivingLicenceRequest) UnmarshalJSON(data []byte) error {
	type Alias DrivingLicenceRequest
	aux := &struct {
		DateOfBirth string `json:"dateOfBirth"`
		Gender      string `json:"gender"`
		*Alias
	}{
		Alias: (*Alias)(dlr),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	dateOfBirth, err := time.Parse("2006-01-02", aux.DateOfBirth)
	if err != nil {
		return err
	}

	dlr.DateOfBirth = dateOfBirth
	dlr.Gender = parseGender(aux.Gender)
	return nil
}

type DrivingLicenceResponse struct {
	FullName             string    `json:"fullName"`
	DrivingLicenceNumber string    `json:"drivingLicenceNumber"`
	DateOfBirth          time.Time `json:"dateOfBirth"`
	Gender               Gender    `json:"gender"`
	Address              string    `json:"address"`
	LicenceValidFrom     time.Time `json:"licenceValidFrom"`
	LicenceValidTo       time.Time `json:"licenceValidTo"`
	LicenceIssueNo       string    `json:"licenceIssueNumber"`
}

func (u DrivingLicenceResponse) MarshalJSON() ([]byte, error) {
	type Alias DrivingLicenceResponse

	gender := "UNKNOWN"
	if u.Gender == Male {
		gender = "MALE"
	} else if u.Gender == Female {
		gender = "FEMALE"
	}
	return json.Marshal(&struct {
		LicenceValidFrom string `json:"licenceValidFrom"`
		LicenceValidTo   string `json:"licenceValidTo"`
		DateOfBirth      string `json:"dateOfBirth"`
		Gender           string `json:"gender"`
		Alias
	}{
		LicenceValidFrom: u.LicenceValidFrom.Format("2006-01-02"),
		LicenceValidTo:   u.LicenceValidTo.Format("2006-01-02"),
		DateOfBirth:      u.DateOfBirth.Format("2006-01-02"),
		Gender:           gender,
		Alias:            (Alias)(u),
	})
}

func main() {

	// API routes
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

		decoder := json.NewDecoder(r.Body)
		var t DrivingLicenceRequest
		err := decoder.Decode(&t)
		if err != nil {
			panic(err)
		}

		drivingLicenceNumber, err := getDrivingLicenceNumber(t)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Print(w, err.Error())
			return
		}
		encoder := json.NewEncoder(w)
		encoder.Encode(drivingLicenceNumber)
	})
	port := ":5000"
	fmt.Println("Server is running on port" + port)

	// Start server on port specified above
	log.Fatal(http.ListenAndServe(port, nil))

}

func getDrivingLicenceNumber(request DrivingLicenceRequest) (DrivingLicenceResponse, error) {
	jar, err := cookiejar.New(&cookiejar.Options{})
	if err != nil {
		panic(err)
	}
	client := &http.Client{
		Jar: jar,
	}

	pesel, err := fetchPesel(client)
	if err != nil {
		return DrivingLicenceResponse{}, err
	}

	drivingLicenceNumber, err := fetchDrivingLicenceNumber(
		client,
		pesel,
		request)

	if err != nil {
		return DrivingLicenceResponse{}, err
	}
	return drivingLicenceNumber, nil
}

func fetchPesel(httpClient *http.Client) (string, error) {
	// resp, err := http.Get("https://www.viewdrivingrecord.service.gov.uk/driving-record/personal-details")
	// if err != nil {
	// 	return "", err
	// }
	// defer resp.Body.Close()

	// r, err := charset.NewReader(resp.Body, resp.Header.Get("Content-Type"))
	// if err != nil {
	// 	return "", err
	// }
	// doc, err := html.Parse(r)

	// if err != nil {
	// 	return "", err
	// }

	// node := htmlquery.FindOne(doc, `//*[@id="pesel"]`)

	// pesel := htmlquery.SelectAttr(node, "value")

	// return pesel, nil

	// for some reason, using this hardcoded value works more reliably
	return "f36ab3a6916d131953e2cb48a5243e7b", nil
}

func fetchDrivingLicenceNumber(httpClient *http.Client, pesel string, request DrivingLicenceRequest) (DrivingLicenceResponse, error) {

	requestUrl := "https://www.viewdrivingrecord.service.gov.uk/driving-record/personal-details"
	method := "POST"

	yyyy, mm, dd := request.DateOfBirth.Date()

	payload := strings.NewReader(url.Values{
		"applicantPassportNumber": {""},
		"pesel":                   {pesel},
		"nino":                    {request.Nino},
		"forename":                {request.Forename},
		"surname":                 {request.Surname},
		"postcode":                {request.Postcode},
		"gender":                  {strconv.Itoa(int(request.Gender))},
		"dob.day":                 {strconv.Itoa(dd)},
		"dob.month":               {strconv.Itoa(int(mm))},
		"dob.year":                {strconv.Itoa(yyyy)},
		"dwpPermission":           {"1"},
	}.Encode())
	fmt.Println(payload)
	req, err := http.NewRequest(method, requestUrl, payload)
	if err != nil {
		return DrivingLicenceResponse{}, err
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	res, err := httpClient.Do(req)
	if err != nil {
		return DrivingLicenceResponse{}, err
	}

	defer res.Body.Close()

	r, err := charset.NewReader(res.Body, res.Header.Get("Content-Type"))
	if err != nil {
		return DrivingLicenceResponse{}, err
	}
	doc, err := html.Parse(r)

	if err != nil {
		return DrivingLicenceResponse{}, err
	}

	dlnNode := htmlquery.FindOne(doc, `//*[@class="dln-field"]`)
	if dlnNode == nil {
		return DrivingLicenceResponse{}, errors.New("unable to find driving licence")
	}
	number := htmlquery.InnerText(dlnNode)

	validFrom, err := extractDate(doc, "licence-valid-from-field")
	if err != nil {
		return DrivingLicenceResponse{}, err
	}

	validTo, err := extractDate(doc, "licence-valid-to-field")
	if err != nil {
		return DrivingLicenceResponse{}, err
	}

	dateOfBirth, err := extractDate(doc, "dob-field")
	if err != nil {
		return DrivingLicenceResponse{}, err
	}

	fullNameNode := htmlquery.FindOne(doc, `//*[@class="name"]`)
	if fullNameNode == nil {
		return DrivingLicenceResponse{}, errors.New("unable to find full name")
	}
	fullName := htmlquery.InnerText(fullNameNode)

	addressNode := htmlquery.FindOne(doc, `//*[@class="address-field"]`)
	if addressNode == nil {
		return DrivingLicenceResponse{}, errors.New("unable to find address")
	}
	address := htmlquery.InnerText(addressNode)

	licenceIssueNumberNode := htmlquery.FindOne(doc, `//*[@class="issue-number-field"]`)
	if licenceIssueNumberNode == nil {
		return DrivingLicenceResponse{}, errors.New("unable to find license issue number")
	}
	licenceIssueNumber := htmlquery.InnerText(licenceIssueNumberNode)

	genderNode := htmlquery.FindOne(doc, `//*[@class="gender-field"]`)
	if genderNode == nil {
		return DrivingLicenceResponse{}, errors.New("unable to find gender")
	}
	genderString := htmlquery.InnerText(genderNode)
	gender := parseGender(genderString)

	response := DrivingLicenceResponse{
		DrivingLicenceNumber: number,
		LicenceValidFrom:     validFrom,
		LicenceValidTo:       validTo,
		DateOfBirth:          dateOfBirth,
		FullName:             fullName,
		Address:              address,
		LicenceIssueNo:       licenceIssueNumber,
		Gender:               gender,
	}
	return response, nil
}

func parseGender(genderString string) Gender {
	if strings.ToUpper(genderString) == "MALE" {
		return Male
	}
	if strings.ToUpper(genderString) == "FEMALE" {
		return Female
	}
	return Unknown
}

func extractDate(doc *html.Node, fieldClass string) (time.Time, error) {
	dateNome := htmlquery.FindOne(doc, fmt.Sprintf(`//*[@class="%s"]`, fieldClass))
	if dateNome == nil {
		return time.Time{}, errors.New("unable to find valid from")
	}
	date, err := time.Parse("02 Jan 2006", htmlquery.InnerText(dateNome))
	if err != nil {
		return time.Time{}, err
	}
	return date, nil
}
