package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

// ItemStruct is for the one item from the receipt
type ItemStruct struct {
	ShortDescription *string
	Price            *string
}

// ReceiptStruct is for the receipt
type ReceiptStruct struct {
	Retailer     *string
	PurchaseDate *string
	PurchaseTime *string
	Items        *[]ItemStruct
	Total        *string
}

// ReceiptIDResponse is for the response with the receipt ID
type ReceiptIDResponse struct {
	Id string
}

// PointsResponse is for the response with the points
type PointsResponse struct {
	Points int64
}

// The array with the processed receipts
var receipts = make(map[string]*ReceiptStruct)

// Checks whether the provided receipt was processed
func wasReceiptProcessed(id string) bool {
	return receipts[id] != nil
}

// The handler for the process receipt route
func processReceipt(w http.ResponseWriter, r *http.Request) {
	// Checking whether this is the POST method
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	// Decoding the JSON into the struct
	decoder := json.NewDecoder(r.Body)
	rs := &ReceiptStruct{}
	err := decoder.Decode(rs)
	// Checking for the missing fields
	if err != nil ||
		rs.Retailer == nil ||
		rs.PurchaseDate == nil ||
		rs.PurchaseTime == nil ||
		rs.Items == nil ||
		rs.Total == nil {
		http.Error(w, "The receipt is invalid.", http.StatusBadRequest)
		fmt.Printf("Invalid receipt: missing fields\n")
		return
	}
	// Checking whether the retailer matches the pattern
	matchRetailer, _ := regexp.MatchString("^[\\w\\s\\-&]+$", *rs.Retailer)
	if !matchRetailer {
		http.Error(w, "The receipt is invalid.", http.StatusBadRequest)
		fmt.Printf("Invalid receipt: invalid retailer\n")
		return
	}
	// Checking whether the purchase date is in the correct format
	_, errPurchaseDate := time.Parse("2006-01-02", *rs.PurchaseDate)
	if errPurchaseDate != nil {
		http.Error(w, "The receipt is invalid.", http.StatusBadRequest)
		fmt.Printf("Invalid receipt: invalid purchase date\n")
		return
	}
	// Checking whether the purchase time is in the correct format
	_, errPurchaseTime := time.Parse("15:04", *rs.PurchaseTime)
	if errPurchaseTime != nil {
		http.Error(w, "The receipt is invalid.", http.StatusBadRequest)
		fmt.Printf("Invalid receipt: invalid purchase time\n")
		return
	}
	// Checking whether there are some items
	if len(*rs.Items) < 1 {
		http.Error(w, "The receipt is invalid.", http.StatusBadRequest)
		fmt.Printf("Invalid receipt: no items\n")
		return
	}
	// Checking the items
	shortDescriptionRegExp, _ := regexp.Compile(`^[\w\s\-]+$`)
	priceRegExp, _ := regexp.Compile(`^\d+\.\d{2}$`)
	for _, item := range *rs.Items {
		// Checking whether the all fields are filled
		if item.ShortDescription == nil ||
			item.Price == nil {
			http.Error(w, "The receipt is invalid.", http.StatusBadRequest)
			fmt.Printf("Invalid receipt: item with missing fields\n")
			return
		}
		// Checking the short description
		if !shortDescriptionRegExp.MatchString(*item.ShortDescription) {
			http.Error(w, "The receipt is invalid.", http.StatusBadRequest)
			fmt.Printf("Invalid receipt: invalid short description in the item\n")
			return
		}
		// Checking the price
		if !priceRegExp.MatchString(*item.Price) {
			http.Error(w, "The receipt is invalid.", http.StatusBadRequest)
			fmt.Printf("Invalid receipt: invalid price in the item\n")
			return
		}
	}

	// Generating the new unique id
	var id uuid.UUID
	for id = uuid.New(); wasReceiptProcessed(id.String()); id = uuid.New() {
	}
	// Saving the receipt
	receipts[id.String()] = rs

	// Preparing the response
	rir := ReceiptIDResponse{
		Id: id.String(),
	}
	// Writing the response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	err = json.NewEncoder(w).Encode(rir)
	if err != nil {
		fmt.Printf("Error encoding JSON: %s\n", err)
	}
}

// The handler for the points awarded for the receipt route
func getReceiptPoints(w http.ResponseWriter, r *http.Request) {
	// Checking whether this is the GET method
	if r.Method != http.MethodGet {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	id := r.PathValue("id")
	// Checking whether the id matches the pattern
	matchId, _ := regexp.MatchString("^\\S+$", id)
	if !matchId {
		http.Error(w, "No receipt found for that ID.", http.StatusNotFound)
		fmt.Printf("Invalid receipt id\n")
		return
	}
	// Checking whether the such receipts exists
	if !wasReceiptProcessed(id) {
		http.Error(w, "No receipt found for that ID.", http.StatusNotFound)
		fmt.Printf("No receipt with the such id\n")
		return
	}

	// Counting the points
	var points int64 = 0
	receipt := receipts[id]
	// One point for every alphanumeric character in the retailer name
	for _, char := range *receipt.Retailer {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			points++
		}
	}
	// 50 points if the total is a round dollar amount with no cents
	total, _ := strconv.ParseFloat(*receipt.Total, 64)
	if math.Mod(total, 1) == 0 {
		points += 50
	}
	// 25 points if the total is a multiple of `0.25`.
	if math.Mod(total, 0.25) == 0 {
		points += 25
	}
	// 5 points for every two items on the receipt
	points += int64((len(*receipt.Items) / 2) * 5)
	// If the trimmed length of the item description is a multiple of 3, multiply the price by `0.2` and round up to the nearest integer. The result is the number of points earned
	for _, item := range *receipt.Items {
		// The trimmed length of the item description
		trimmedLength := len(strings.TrimSpace(*item.ShortDescription))
		if trimmedLength%3 == 0 {
			price, _ := strconv.ParseFloat(*item.Price, 64)
			points += int64(math.Ceil(price * 0.2))
		}
	}
	// 6 points if the day in the purchase date is odd
	parsedDate, _ := time.Parse("2006-01-02", *receipt.PurchaseDate)
	if parsedDate.Day()%2 != 0 {
		points += 6
	}
	// 10 points if the time of purchase is after 2:00pm and before 4:00pm
	parsedTime, _ := time.Parse("15:04", *receipt.PurchaseTime)
	if parsedTime.Hour() >= 14 && parsedTime.Hour() < 16 {
		points += 10
	}

	// Preparing the response
	pr := PointsResponse{
		Points: points,
	}
	// Writing the response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	err := json.NewEncoder(w).Encode(pr)
	if err != nil {
		fmt.Printf("Error encoding response: %v\n", err)
	}
}

// The entry point of the application
func main() {
	// Setting the handlers for the routes
	http.HandleFunc("/receipts/process", processReceipt)
	http.HandleFunc("/receipts/{id}/points", getReceiptPoints)

	// Starting the server
	err := http.ListenAndServe(":80", nil)
	if errors.Is(err, http.ErrServerClosed) {
		fmt.Printf("server closed\n")
	} else if err != nil {
		fmt.Printf("error starting server: %s\n", err)
		os.Exit(1)
	}
}
