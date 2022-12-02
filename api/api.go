package api

import (
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/h0rzn/monitoring_agent/api/hub"
	"github.com/h0rzn/monitoring_agent/dock/controller"
)

var upgrade = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

type API struct {
	Router     *gin.Engine
	Addr       string
	Controller *controller.Controller
	Hub        *hub.Hub
}

func NewAPI(addr string) (*API, error) {
	ctrl, err := controller.NewController()
	if err != nil {
		return &API{}, err
	}
	return &API{
		Router:     gin.Default(),
		Addr:       addr,
		Controller: ctrl,
		Hub:        hub.NewHub(ctrl),
	}, nil
}

func (api *API) RegRoutes() {
	api.Router.Use(cors.Default())
	api.Router.GET("/containers/:id", api.Container)
	api.Router.GET("/containers/all", api.Containers)
	api.Router.GET("/containers/:id/metrics", api.Metrics)
	api.Router.GET("/stream", api.Stream)
}

func (api *API) Run() {
	api.Controller.Init()
	go api.Hub.Run()
	api.Router.Run(api.Addr)

}
