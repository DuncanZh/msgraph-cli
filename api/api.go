package api

import (
	"context"
	"errors"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/abiosoft/ishell/v2"
	abstractions "github.com/microsoft/kiota-abstractions-go"
	auth "github.com/microsoft/kiota-authentication-azure-go"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	msgraphcore "github.com/microsoftgraph/msgraph-sdk-go-core"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/models/odataerrors"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

type GraphAPI struct {
	credential      *azidentity.ClientSecretCredential
	userClient      *msgraphsdk.GraphServiceClient
	graphUserScopes []string
}

type T interface{}

func NewGraphAPI() *GraphAPI {
	g := &GraphAPI{}
	return g
}

func (g *GraphAPI) InitializeGraphForUserAuth(clientId string, clientSecret string, tenantId string) error {
	g.graphUserScopes = []string{"https://graph.microsoft.com/.default"}

	credential, err := azidentity.NewClientSecretCredential(tenantId, clientId, clientSecret, nil)
	if err != nil {
		return err
	}

	g.credential = credential

	// Create an auth provider using the credential
	authProvider, err := auth.NewAzureIdentityAuthenticationProviderWithScopes(credential, g.graphUserScopes)
	if err != nil {
		return err
	}

	// Create a request adapter using the auth provider
	adapter, err := msgraphsdk.NewGraphRequestAdapter(authProvider)
	if err != nil {
		return err
	}

	// Create a Graph client using request adapter
	client := msgraphsdk.NewGraphServiceClient(adapter)
	g.userClient = client

	return nil
}

func (g *GraphAPI) ListResource(resource string, expand []string) []map[string]interface{} {
	// Separate resource path
	resources := strings.Split(resource, "/")
	for i := 0; i < len(resources); i++ {
		resources[i] = strings.Title(resources[i])
	}

	// Get the corresponding method recursively
	// Equivalent to g.userClient.Method1().Method2.Get(context.Background(), nil)
	method := reflect.ValueOf(g.userClient)

	for _, v := range resources {
		method = method.MethodByName(v)
		if !method.IsValid() {
			fmt.Println("Error: Unknown resource " + v)
			return nil
		}
		method = method.Call([]reflect.Value{})[0]
	}
	method = method.MethodByName("Get")

	//Create config for expand, equivalent to
	//cfg := &users.UsersRequestBuilderGetRequestConfiguration{
	//	QueryParameters: &users.UsersRequestBuilderGetQueryParameters{
	//		Expand: expand,
	//	},
	//}
	cfgType := method.Type().In(1)
	cfg := reflect.Zero(cfgType)
	if len(expand) == 1 {
		cfg = reflect.New(cfgType.Elem())
		queryType := cfg.Elem().FieldByName("QueryParameters").Type()
		query := reflect.New(queryType.Elem())
		query.Elem().FieldByName("Expand").Set(reflect.ValueOf(expand))
		cfg.Elem().FieldByName("QueryParameters").Set(query)
	}

	// Call the method with default context and the required type
	// Get the type of the second argument and create a nil pointer
	resp := method.Call([]reflect.Value{reflect.ValueOf(context.Background()), cfg})

	// Check error, the second return value
	if v := resp[1].Interface(); v != nil {
		g.printError(v.(error))
		return nil
	}

	// Iterate the collection using the base type
	var results []map[string]interface{}
	pageIterator, err := msgraphcore.NewPageIterator[models.DirectoryObjectable](resp[0].Interface(), g.userClient.GetAdapter(), models.CreateDirectoryObjectCollectionResponseFromDiscriminatorValue)
	err = pageIterator.Iterate(context.Background(), func(item models.DirectoryObjectable) bool {
		// Handle the expand argument
		for _, v := range expand {
			retrieveRelatedResource(&item, v)
		}
		// Append result
		results = append(results, item.GetBackingStore().Enumerate())
		// Escape from the current iteration
		return true
	})
	if err != nil {
		g.printError(err)
	}

	return results
}

func (g *GraphAPI) GetResourceByUserIdsConcurrent(c *ishell.Context, userIds []string, resource string, expand []string) map[string][]interface{} {
	result := make(map[string][]interface{})
	lock := sync.Mutex{}

	slice := 20
	workers := 4

	input := make(chan []string, len(userIds)/slice+1)
	output := make(chan bool, len(userIds))
	pause := make(chan int, 2)

	resources := strings.Split(resource, "/")
	for i := 0; i < len(resources); i++ {
		resources[i] = strings.Title(resources[i])
	}
	resource = strings.Join(resources, "")

	method := reflect.ValueOf(g.userClient.Users().ByUserId(userIds[0]))
	for _, v := range resources {
		if v == "Inbox" {
			method = method.MethodByName("ByMailFolderId")
			if !method.IsValid() {
				fmt.Println("Error: Unknown resource " + v)
				return nil
			}
			method = method.Call([]reflect.Value{reflect.ValueOf("inbox")})[0]
		} else {
			method = method.MethodByName(v)
			if !method.IsValid() {
				fmt.Println("Error: Unknown resource " + v)
				return nil
			}
			method = method.Call([]reflect.Value{})[0]
		}
	}
	cfgType := method.MethodByName("ToGetRequestInformation").Type().In(1)
	cfg := reflect.New(cfgType.Elem())
	if len(expand) == 1 {
		queryType := cfg.Elem().FieldByName("QueryParameters").Type()
		query := reflect.New(queryType.Elem())
		query.Elem().FieldByName("Expand").Set(reflect.ValueOf(expand))
		cfg.Elem().FieldByName("QueryParameters").Set(query)
	}

	for i := 0; i < workers; i++ {
		go g.GetResourceByUserIdsWorker(resources, cfg, input, output, pause, &lock, &result)
	}

	i := 0
	for ; i < len(userIds)-slice; i += slice {
		input <- userIds[i : i+slice]
	}
	input <- userIds[i:]

	c.ProgressBar().Start()

	t := 0
	for len(output) != len(userIds) {
		percent := len(output) * 100 / len(userIds)

		if len(pause) == 2 {
			t = <-pause
			if t == -1 {
				c.ProgressBar().Stop()
				fmt.Println("Error: Unexpected response")
				return nil
			}
		}
		if len(pause) == 1 {
			c.ProgressBar().Suffix(fmt.Sprint(" ", len(output), "/", len(userIds), " (", percent, "%) PAUSED: Too many requests, please wait for ", t, " seconds..."))
			c.ProgressBar().Progress(percent)
		} else {
			c.ProgressBar().Suffix(fmt.Sprint(" ", len(output), "/", len(userIds), " (", percent, "%)", "                                                            "))
			c.ProgressBar().Progress(percent)
		}
	}

	c.ProgressBar().Suffix(fmt.Sprint(" ", len(userIds), "/", len(userIds), " (", 100, "%)", "                                          "))
	c.ProgressBar().Progress(100)
	c.ProgressBar().Stop()

	return result
}

func (g *GraphAPI) GetResourceByUserIdsWorker(resources []string, configuration reflect.Value, input chan []string, output chan bool, pause chan int, lock *sync.Mutex, result *map[string][]interface{}) {
retry:
	for userIds := range input {
		batch := msgraphcore.NewBatchRequest(g.userClient.GetAdapter())
		stepMap := make(map[string]msgraphcore.BatchItem)

		for _, id := range userIds {
			if stepMap[id] != nil {
				output <- true
			}

			method := reflect.ValueOf(g.userClient.Users().ByUserId(id))
			for _, v := range resources {
				if v == "Inbox" {
					method = method.MethodByName("ByMailFolderId")
					method = method.Call([]reflect.Value{reflect.ValueOf("inbox")})[0]
				} else {
					method = method.MethodByName(v)
					method = method.Call([]reflect.Value{})[0]
				}
			}
			request := method.MethodByName("ToGetRequestInformation").Call([]reflect.Value{reflect.ValueOf(context.Background()), configuration})[0].Interface().(*abstractions.RequestInformation)

			step, err := batch.AddBatchRequestStep(*request)
			if err != nil {
				fmt.Println("\n" + err.Error())
				input <- userIds
				return
			}
			stepMap[id] = step
		}

		for len(pause) > 0 {
			// Wait if paused
		}

		resp, err := batch.Send(context.Background(), g.userClient.GetAdapter())
		if err != nil {
			fmt.Println("\n" + err.Error())
			input <- userIds
			return
		}

		for k, v := range stepMap {
			response, err := msgraphcore.GetBatchResponseById[models.BaseItemCollectionResponseable](resp, *v.GetId(), models.CreateBaseItemCollectionResponseFromDiscriminatorValue)
			if err != nil {
				if strings.Contains(err.Error(), "429") {
					if len(pause) == 0 {
						t, _ := strconv.ParseInt(resp.GetResponseById(*v.GetId()).GetHeaders()["Retry-After"], 10, 32)
						pause <- int(t)
						pause <- int(t)
						time.Sleep(time.Duration(t) * time.Second)
						<-pause
					}
					input <- userIds
					continue retry
				} else if strings.Contains(err.Error(), "404") {
					output <- true
					continue
				} else {
					g.printError(err)
					pause <- -1
					pause <- -1
					return
				}
			}

			lock.Lock()
			for _, v := range response.GetValue() {
				(*result)[k] = append((*result)[k], v.GetBackingStore().Enumerate())
			}
			lock.Unlock()

			output <- true
		}
	}
}

func (g *GraphAPI) Test() {
	resp, err := g.userClient.Users().Get(context.Background(), nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	pageIterator, err := msgraphcore.NewPageIterator[models.DirectoryObjectable](resp, g.userClient.GetAdapter(), models.CreateDirectoryObjectCollectionResponseFromDiscriminatorValue)
	err = pageIterator.Iterate(context.Background(), func(item models.DirectoryObjectable) bool {
		return true
	})
	if err != nil {
		g.printError(err)
	}
	/*
		for _, v := range resp.GetValue() {
			fmt.Printf("%v", v.GetBackingStore().Enumerate())
		}
	*/
	/*
		pageIterator, err := msgraphcore.NewPageIterator[models.Entityable](resp, g.userClient.GetAdapter(), models.CreateEntityFromDiscriminatorValue)
		err = pageIterator.Iterate(context.Background(), func(item models.Entityable) bool {
			return true
		})
		if err != nil {
			g.printError(err)
		}
	*/
}

func (g *GraphAPI) IsInitiated() bool {
	return g.userClient != nil
}

func (g *GraphAPI) printError(err error) {
	var ODataError *odataerrors.ODataError
	switch {
	case errors.As(err, &ODataError):
		errors.As(err, &ODataError)
		fmt.Printf("\nError: %s\n", ODataError.Error())
		if terr := ODataError.GetErrorEscaped(); terr != nil {
			fmt.Printf("Code: %s\n", *terr.GetCode())
			fmt.Printf("Message: %s\n", *terr.GetMessage())
		}
	default:
		fmt.Println("\nError: " + err.Error())
	}
}

func retrieveRelatedResource(item *models.DirectoryObjectable, resource string) {
	m, _ := (*item).GetBackingStore().Get(resource)

	var a []models.DirectoryObjectable
	arr := reflect.ValueOf(m)
	for i := 0; i < arr.Len(); i++ {
		a = append(a, arr.Index(i).Interface().(models.DirectoryObjectable))
	}

	var result []map[string]interface{}
	for _, v := range a {
		result = append(result, v.GetBackingStore().Enumerate())
	}

	m = result
	(*item).GetBackingStore().Set(resource, m)
}
