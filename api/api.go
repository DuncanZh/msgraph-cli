package api

import (
	"context"
	"errors"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/abiosoft/ishell"
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

func (g *GraphAPI) GetUsers() []map[string]interface{} {
	result, err := g.userClient.Users().Get(context.Background(), nil)
	if err != nil {
		g.printError(err)
		return nil
	}

	var results []map[string]interface{}
	pageIterator, err := msgraphcore.NewPageIterator[models.Userable](result, g.userClient.GetAdapter(), models.CreateUserCollectionResponseFromDiscriminatorValue)
	err = pageIterator.Iterate(context.Background(), func(user models.Userable) bool {
		results = append(results, user.GetBackingStore().Enumerate())
		return true
	})
	return results
}

func GetResourceByUserIdsConcurrent(c *ishell.Context, userIds []string, resource string, configuration T, n int, slice int) map[string][]interface{} {
	result := make(map[string][]interface{})
	lock := sync.Mutex{}

	input := make(chan []string, len(userIds)/slice+1)
	output := make(chan bool, len(userIds))
	pause := make(chan int, 2)

	g := c.Get("api").(*GraphAPI)

	resources := strings.Split(resource, "/")
	for i := 0; i < len(resources); i++ {
		resources[i] = strings.Title(resources[i])
	}

	for i := 0; i < n; i++ {
		go GetResourceByUserIdsWorker(g, resources, configuration, input, output, pause, &lock, &result)
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

func GetResourceByUserIdsWorker(g *GraphAPI, resources []string, configuration T, input chan []string, output chan bool, pause chan int, lock *sync.Mutex, result *map[string][]interface{}) {
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
				method = method.MethodByName(v).Call([]reflect.Value{})[0]
			}
			request := method.MethodByName("ToGetRequestInformation").Call([]reflect.Value{reflect.ValueOf(context.Background()), reflect.ValueOf(configuration)})[0].Interface().(*abstractions.RequestInformation)

			step, err := batch.AddBatchRequestStep(*request)
			if err != nil {
				input <- userIds
				continue retry
			}
			stepMap[id] = step
		}

		for len(pause) > 0 {
			// Wait if paused
		}

		resp, err := batch.Send(context.Background(), g.userClient.GetAdapter())
		if err != nil {
			input <- userIds
			continue retry
		}

		for k, v := range stepMap {
			response, err := msgraphcore.GetBatchResponseById[models.BaseItemCollectionResponseable](resp, *v.GetId(), models.CreateBaseItemCollectionResponseFromDiscriminatorValue)
			if err != nil {
				if strings.Contains(err.Error(), "429") && len(pause) == 0 {
					t, _ := strconv.ParseInt(resp.GetResponseById(*v.GetId()).GetHeaders()["Retry-After"], 10, 32)
					pause <- int(t)
					pause <- int(t)
					time.Sleep(time.Duration(t) * time.Second)
					<-pause
				}
				input <- userIds
				continue retry
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

func (g *GraphAPI) IsInitiated() bool {
	return g.userClient != nil
}

func (g *GraphAPI) printError(err error) {
	var ODataError *odataerrors.ODataError
	switch {
	case errors.As(err, &ODataError):
		errors.As(err, &ODataError)
		fmt.Printf("error: %s\n", ODataError.Error())
		if terr := ODataError.GetErrorEscaped(); terr != nil {
			fmt.Printf("code: %s\n", *terr.GetCode())
			fmt.Printf("msg: %s\n", *terr.GetMessage())
		}
	}
}
