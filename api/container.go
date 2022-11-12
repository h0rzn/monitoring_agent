package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/h0rzn/monitoring_agent/api/ws"
	"github.com/h0rzn/monitoring_agent/dock"
)

type KeepAliveMsg struct {
	KeepAlive bool `json:"keep_alive"`
}

func (api *API) Container(ctx *gin.Context) {
	id := ctx.Param("id")
	container := api.Controller.Container(id)
	if container == (&dock.Container{}) {
		HttpErr(ctx, http.StatusNotFound, errors.New("container not found"))
	} else {
		contJson, err := container.MarshalJSON()
		if err != nil {
			HttpErr(ctx, http.StatusInternalServerError, err)
			return
		}
		ctx.Data(http.StatusOK, "application/json; charset=utf-8", contJson)
	}
}

func (api *API) Containers(ctx *gin.Context) {
	b, err := api.Controller.Containers.MarshalJSON()
	if err != nil {
		HttpErr(ctx, http.StatusInternalServerError, errors.New("failed to fetch containers"))
		return
	}
	ctx.Data(http.StatusOK, "application/json; charset=utf-8", b)
}

func (api *API) ContainerMetrics(ctx *gin.Context) {
	id := ctx.Param("id")
	api.metricsWS(ctx.Writer, ctx.Request, id)
}

func (api *API) ContainerLogs(ctx *gin.Context) {
	id := ctx.Param("id")
	api.logsWS(ctx.Writer, ctx.Request, id)
}

func (api *API) metricsWS(w http.ResponseWriter, r *http.Request, id string) {
	con, err := upgrade.Upgrade(w, r, nil)
	if err != nil {
		errBytes, _ := HttpErrBytes(500, err)
		w.Write(errBytes)
		return
	}
	container := api.Controller.Container(id)
	if container == (&dock.Container{}) {
		errBytes, _ := HttpErrBytes(404, errors.New("container not found"))
		w.Write(errBytes)
		return
	}

	done := make(chan bool)
	metrics := container.MetricsStream(done)

	for set := range metrics {
		msg := ws.NewMessage("metric_set", set)
		if err != nil {
			HttpErrBytes(0, err)
			con.Close()
			return
		}
		err = con.WriteJSON(msg)
	}

}

func (api *API) logsWS(w http.ResponseWriter, r *http.Request, id string) {
	con, err := upgrade.Upgrade(w, r, nil)
	if err != nil {
		errBytes, _ := HttpErrBytes(500, err)
		w.Write(errBytes)
		return
	}

	container := api.Controller.Container(id)
	if container == (&dock.Container{}) {
		errBytes, _ := HttpErrBytes(404, errors.New("container not found"))
		w.Write(errBytes)
		return
	}

	done := make(chan bool)
	entries := container.Logs.Stream(done)

	for entry := range entries {
		msg := ws.NewMessage("entry", entry)
		err = con.WriteJSON(msg)
		if err != nil {
			con.Close()
			return
		}
	}
	done <- true
	con.Close()

}
