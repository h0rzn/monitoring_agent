package controller

import (
	"context"
	"fmt"

	dock_events "github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/h0rzn/monitoring_agent/dock/container"
	"github.com/h0rzn/monitoring_agent/dock/controller/db"
	"github.com/h0rzn/monitoring_agent/dock/events"
	"github.com/h0rzn/monitoring_agent/dock/image"
	"github.com/sirupsen/logrus"
)

type Controller struct {
	c *client.Client
	// Storage    *Storage
	DB         *db.DB
	About      *About
	Volumes    []*Volume
	Events     *events.Events
	Containers *container.Storage
	Images     *image.Storage
}

type About struct {
	Version    string `json:"version"`
	APIVersion string `json:"api_version"`
	OS         string `json:"os"`
	ImageN     int    `json:"image_n"`
	ContainerN int    `json:"container_n"`
}
type Volume struct {
	Name       string `json:"name"`
	Mountpoint string `json:"mountpoint"`
	Driver     string `json:"driver"`
	Created    string `json:"created"`
	UsedBy     int64  `json:"used_by"` // used by containers
	Size       int64  `json:"size"`
}

func NewController() (ctr *Controller, err error) {
	c, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, err
	}

	return &Controller{
		c:          c,
		DB:         &db.DB{},
		About:      &About{},
		Volumes:    make([]*Volume, 0),
		Events:     events.NewEvents(c),
		Containers: container.NewStorage(c),
		Images:     image.NewStorage(c),
	}, err
}

func (ctr *Controller) Init() (err error) {
	logrus.Infoln("- CONTROLLER - starting")
	err = ctr.UpdateAbout()
	if err != nil {
		logrus.Warnf("- CONTROLLER - about might not be complete, err: %s\n", err)
	}

	err = ctr.UpdateVolumes()
	if err != nil {
		logrus.Warnf("- CONTROLLER - volumes might not be complete, err: %s\n", err)
	}

	err = ctr.Events.Init()
	if err != nil {
		return err
	}
	go ctr.HandleEvents()

	err = ctr.Images.Init()
	if err != nil {
		logrus.Errorf("- STORAGE - (images) failed to init: %s\n", err)
		return
	}

	err = ctr.Containers.Init(ctr.Images.ByID)
	if err != nil {
		logrus.Errorf("- STORAGE - (containers) failed to init: %s\n", err)
		return
	}
	go func() {
		for items := range ctr.Containers.Broadcast() {
			go ctr.DB.Client.BulkWrite(items)
		}
		fmt.Println("feed writer left")
	}()

	err = ctr.DB.Init()
	if err != nil {
		logrus.Errorf("- STORAGE - (db) failed to init: %s\n", err)
	}
	return err
}

func (ctr *Controller) UpdateAbout() (err error) {
	ctx := context.Background()
	version, err := ctr.c.ServerVersion(ctx)
	if err != nil {
		return
	}
	ctr.About.Version = version.Version
	ctr.About.APIVersion = version.APIVersion
	ctr.About.OS = version.Os

	ctx = context.Background()
	info, err := ctr.c.Info(ctx)
	if err != nil {
		return
	}
	ctr.About.ImageN = info.Images
	ctr.About.ContainerN = info.Containers

	return
}

func (ctr *Controller) UpdateVolumes() (err error) {
	ctx := context.Background()
	volList, err := ctr.c.VolumeList(ctx, filters.Args{})
	if err != nil {
		return
	}

	updated := make([]*Volume, 0)
	for _, v := range volList.Volumes {
		// for some reason usage data can be nil
		var usedBy, size int64
		if v.UsageData == nil {
			usedBy, size = -1, -1
		} else {
			usedBy = v.UsageData.RefCount
			size = v.UsageData.Size
		}

		new := &Volume{
			Name:       v.Name,
			Mountpoint: v.Mountpoint,
			Driver:     v.Driver,
			Created:    v.CreatedAt,
			UsedBy:     usedBy,
			Size:       size,
		}
		updated = append(updated, new)
	}
	ctr.Volumes = updated
	return
}

func (ctr *Controller) HandleEvents() {
	eventRcv, err := ctr.Events.Get()
	if err != nil {
		logrus.Errorf("- CONTROLLER - failed to run event handler: %s\n", err)
		return
	}

	logrus.Infoln("- CONTROLLER - running event handler...")
	for set := range eventRcv.In {
		fmt.Println("handling event")
		event := set.Data.(dock_events.Message)
		// add queue
		if event.Type != dock_events.ContainerEventType {
			continue
		}
		switch event.Status {
		case "start":
			ctr.ContainerStart(event)
		case "stop":
			ctr.ContainerStop(event)
		case "destroy":
			ctr.ContainerDestroy(event)
		default:
			logrus.Warnf("- CONTROLLER - event %s is unkown or not implemented\n", event.Status)
		}
		ctr.UpdateAbout()
	}
}

func (ctr *Controller) ContainerStart(e dock_events.Message) {
	err := ctr.Containers.Add(e.ID)
	logEventExec(err, e)
}

func (ctr *Controller) ContainerStop(e dock_events.Message) {
	err := ctr.Containers.Stop(e.ID)
	logEventExec(err, e)
}

func (ctr *Controller) ContainerDestroy(e dock_events.Message) {
	err := ctr.Containers.Remove(e.ID)
	logEventExec(err, e)
}

func (ctr *Controller) Quit() {
	// complete this
	ctr.c.Close()
	logrus.Infoln("- CONTROLLER - quit")
}

func logEventExec(err error, e dock_events.Message) {
	if err != nil {
		logrus.Errorf("- CONTROLLER - exec of event %s failed: %s\n", e.Status, err)
	} else {
		logrus.Infof("- CONTROLLER - exec of event %s successful\n", e.Status)
	}
}
