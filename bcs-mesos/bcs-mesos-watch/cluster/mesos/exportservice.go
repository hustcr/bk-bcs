/*
 * Tencent is pleased to support the open source community by making Blueking Container Service available.
 * Copyright (C) 2019 THL A29 Limited, a Tencent company. All rights reserved.
 * Licensed under the MIT License (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 * http://opensource.org/licenses/MIT
 * Unless required by applicable law or agreed to in writing, software distributed under
 * the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package mesos

import (
	"bk-bcs/bcs-common/common/blog"
	commtypes "bk-bcs/bcs-common/common/types"
	"bk-bcs/bcs-common/pkg/cache"
	lbtypes "bk-bcs/bcs-common/pkg/loadbalance/v2"
	"bk-bcs/bcs-mesos/bcs-container-executor/container"
	"bk-bcs/bcs-mesos/bcs-mesos-watch/cluster"
	"bk-bcs/bcs-mesos/bcs-mesos-watch/types"
	"bk-bcs/bcs-mesos/bcs-mesos-watch/util"
	schedtypes "bk-bcs/bcs-mesos/bcs-scheduler/src/types"
	"encoding/json"
	"fmt"
	"golang.org/x/net/context"
	"reflect"
	"strconv"
	"strings"
	"time"
)

//NewExportServiceWatch create export service watch
func NewExportServiceWatch(cxt context.Context, client ZkClient, reporter cluster.Reporter, clusterID, watchPath string) *ExportServiceWatch {
	watch := &ExportServiceWatch{
		report:      reporter,
		esCache:     cache.NewCache(epKeyFunc),
		esInfoCache: cache.NewCache(esInfoKeyFunc),
		queue:       make(chan *types.BcsSyncData, 10240),
		ClusterID:   clusterID,
		client:      client,
		basePath:    watchPath,
	}
	go watch.worker(cxt)
	return watch
}

func epKeyFunc(data interface{}) (string, error) {
	eps, ok := data.(*lbtypes.ExportService)
	if !ok {
		return "", fmt.Errorf("Error data type for ExportService")
	}
	return eps.Namespace + "/" + eps.ServiceName, nil
}

func esInfoKeyFunc(data interface{}) (string, error) {
	esInfo, ok := data.(*ExportServiceInfo)
	if !ok {
		return "", fmt.Errorf("Error data type for ExportServiceInfo")
	}
	return esInfo.bcsService.ObjectMeta.NameSpace + "." + esInfo.bcsService.ObjectMeta.Name, nil
}

type ExportServiceInfo struct {
	bcsService    *commtypes.BcsService
	exportService *lbtypes.ExportService
	creatTime     int64
}

//ExportServiceWatch watch for taskGroup
type ExportServiceWatch struct {
	report      cluster.Reporter //reporter to cluster
	esCache     cache.Store      //exportservice info cache
	esInfoCache cache.Store
	queue       chan *types.BcsSyncData //queue for handle goroutine, ensure data consistency
	ClusterID   string
	client      ZkClient //zookeeper client
	basePath    string
}

func (watch *ExportServiceWatch) postData(data *types.BcsSyncData) {
	if data == nil {
		return
	}
	blog.V(3).Infof("post data(type:%s, action:%s) to ExportServiceWatch", data.DataType, data.Action)
	watch.queue <- data
}

func (watch *ExportServiceWatch) worker(cxt context.Context) {
	tick := time.NewTicker(120 * time.Second)
	for {
		select {
		case <-cxt.Done():
			blog.V(3).Infof("ExportServiceWatch asked to exit")
			return
		case <-tick.C:
			blog.V(3).Infof("ExportServiceWatch is running, managed service num: %d", watch.esInfoCache.Num())
		case data := <-watch.queue:
			if data.DataType == "Service" {
				switch data.Action {
				case "Add":
					watch.addService(data.Item.(*commtypes.BcsService))
				case "Delete":
					watch.deleteService(data.Item.(*commtypes.BcsService))
				case "Update":
					watch.updateService(data.Item.(*commtypes.BcsService))
				}
			} else {
				splitType := strings.Split(data.DataType, "_")
				if splitType[0] == "TaskGroup" {
					switch data.Action {
					case "Add":
						watch.addTaskGroup(data.Item.(*schedtypes.TaskGroup))
					case "Delete":
						watch.deleteTaskGroup(data.Item.(*schedtypes.TaskGroup))
					case "Update":
						watch.updateTaskGroup(data.Item.(*schedtypes.TaskGroup))
					}
				} else {
					blog.Warn("ExportServiceWatch recieve unknown data action(type:%s, action:%s)", data.DataType, data.Action)
				}
			}

		}
	}
}

func (watch *ExportServiceWatch) addService(service *commtypes.BcsService) {
	key := service.ObjectMeta.NameSpace + "." + service.ObjectMeta.Name
	blog.Info("ExportServiceWatch recieve addService(%+v)", service)

	groupLabel := service.ObjectMeta.Labels["BCSGROUP"]
	if groupLabel == "" {
		blog.Info("ExportServiceWatch recieve addService(%s), no BCSGROUP label, do nothing", key)
		return
	}

	_, exist, err := watch.esInfoCache.GetByKey(key)
	if err != nil {
		blog.Error("when recieve addService event, get esInfo %s from cache return err:%s", key, err.Error())
	}
	if exist == true {
		blog.V(3).Infof("when receive addService event, esInfo %s is in already in cache, will be updated", key)
	}

	esInfo := watch.createEpServiceInfo(service)
	if esInfo == nil {
		blog.Error("when receive addService %s, create EpServiceInfo fail", key)
		return
	}

	err = watch.SyncExportServiceBackends(esInfo)
	watch.esInfoCache.Add(esInfo)

	if err != nil {
		blog.Error("ExportServiceWatch: service (%s %s) get backends error %s",
			esInfo.bcsService.NameSpace, esInfo.bcsService.Name, err.Error())
		return
	}

	watch.AddEvent(esInfo.exportService)

	blog.Infof("ExportServiceWatch recieve addService(%+v) end", service)

	return
}

func (watch *ExportServiceWatch) deleteService(service *commtypes.BcsService) {
	key := service.ObjectMeta.NameSpace + "." + service.ObjectMeta.Name
	blog.Info("ExportServiceWatch recieve deleteService(%+v)", service)

	groupLabel := service.ObjectMeta.Labels["BCSGROUP"]
	if groupLabel == "" {
		blog.Info("ExportServiceWatch recieve deleteService(%s), no BCSGROUP label, do nothing", key)
		return
	}

	cacheData, exist, err := watch.esInfoCache.GetByKey(key)
	if exist == false || err != nil {
		blog.V(3).Infof("when receive deleteService event, esInfo %s is not in cache or error", key)
		es := new(lbtypes.ExportService)
		es.ObjectMeta = service.ObjectMeta
		es.Cluster = service.ObjectMeta.Labels["io.tencent.bcs.cluster"]
		es.Namespace = service.ObjectMeta.NameSpace
		es.ServiceName = service.ObjectMeta.Name
		watch.DeleteEvent(es)
		return
	}
	esInfo, ok := cacheData.(*ExportServiceInfo)
	if !ok {
		blog.V(3).Infof("when receive deleteService event, convert cachedata to ExportServiceInfo fail, key(%s)", key)
		es := new(lbtypes.ExportService)
		es.ObjectMeta = service.ObjectMeta
		es.Cluster = service.ObjectMeta.Labels["io.tencent.bcs.cluster"]
		es.Namespace = service.ObjectMeta.NameSpace
		es.ServiceName = service.ObjectMeta.Name
		watch.DeleteEvent(es)
		return
	}

	watch.DeleteEvent(esInfo.exportService)
	watch.esInfoCache.Delete(esInfo)
	return
}

// maybe we can do different work for different changed contents
func (watch *ExportServiceWatch) updateService(service *commtypes.BcsService) {
	key := service.ObjectMeta.NameSpace + "." + service.ObjectMeta.Name
	blog.V(3).Infof("ExportServiceWatch recieve updateService(%+v)", service)

	groupLabel := service.ObjectMeta.Labels["BCSGROUP"]
	if groupLabel == "" {
		blog.Info("ExportServiceWatch recieve updateService(%s), no BCSGROUP label, do nothing", key)
		return
	}

	cacheData, exist, err := watch.esInfoCache.GetByKey(key)
	if err != nil {
		blog.V(3).Infof("when receive updateService event, get esInfo %s from cache return err:%s", key, err.Error())
	}
	if exist == false {
		blog.V(3).Infof("when receive updateService event, esInfo %s is in not in cache, will be added", key)
	} else {
		esInfo, ok := cacheData.(*ExportServiceInfo)
		if ok {
			if reflect.DeepEqual(esInfo.bcsService, service) {
				blog.V(3).Infof("when receive updateService %s, service content not changed, report exportService", key)

				watch.UpdateEvent(esInfo.exportService)

				return
			}
			blog.Info("when receive updateService %s, update to new service: %+v ", key, service)
		} else {
			blog.V(3).Infof("when receive updateService %s, cache data error, to update to new service: %+v ", key, service)
		}
	}

	esInfo := watch.createEpServiceInfo(service)
	if esInfo == nil {
		blog.Error("when receive updateService %s, create EpServiceInfo fail", key)
		return
	}
	watch.esInfoCache.Add(esInfo)
	watch.UpdateEvent(esInfo.exportService)
	return
}

func (watch *ExportServiceWatch) createEpServiceInfo(service *commtypes.BcsService) *ExportServiceInfo {
	key := service.ObjectMeta.NameSpace + "." + service.ObjectMeta.Name
	if service == nil {
		return nil
	}

	esInfo := new(ExportServiceInfo)
	esInfo.bcsService = service
	esInfo.exportService = new(lbtypes.ExportService)
	esInfo.exportService.ObjectMeta = service.ObjectMeta
	esInfo.exportService.Cluster = service.ObjectMeta.Labels["io.tencent.bcs.cluster"]
	esInfo.exportService.Namespace = service.ObjectMeta.NameSpace
	esInfo.exportService.ServiceName = service.ObjectMeta.Name

	//esInfo.exportService.BCSGroup = append(esInfo.exportService.BCSGroup, service.ObjectMeta.Labels["BCSGROUP"])
	splitGroups := strings.Split(service.ObjectMeta.Labels["BCSGROUP"], ",")
	for _, oneGroup := range splitGroups {
		esInfo.exportService.BCSGroup = append(esInfo.exportService.BCSGroup, oneGroup)
	}

	esInfo.exportService.Balance = service.ObjectMeta.Labels["BCSBALANCE"]
	esInfo.exportService.ServiceWeight = make(map[string]int)
	for k, v := range service.ObjectMeta.Labels {
		blog.V(3).Infof("check service weight label: service(%s)  label(%s -> %s)", key, k, v)
		if strings.HasPrefix(k, "BCS-WEIGHT-") {
			w, _ := strconv.Atoi(v)
			rs := []rune(k)
			lenth := len(rs)
			realK := string(rs[11:lenth])
			esInfo.exportService.ServiceWeight[realK] = w
			blog.V(3).Infof("service %s weight config, label: %s -> %d", key, realK, w)
		}
	}
	for index, onePort := range service.Spec.Ports {
		blog.V(3).Infof("service(%s) export port[%d]: %s %s %s %d", key, index, onePort.Name, onePort.DomainName, onePort.Protocol, onePort.Port)
		exPort := lbtypes.ExportPort{
			Name:        onePort.Name,
			Path:        onePort.Path,
			Protocol:    onePort.Protocol,
			BCSVHost:    onePort.DomainName,
			ServicePort: onePort.Port,
		}
		esInfo.exportService.ServicePort = append(esInfo.exportService.ServicePort, exPort)
	}
	esInfo.creatTime = time.Now().Unix()
	blog.Info("ExportServiceWatch: exportservice created: %+v ", esInfo.exportService)

	return esInfo
}

func (watch *ExportServiceWatch) getTaskGroupServiceLabel(service *commtypes.BcsService, tskgroup *schedtypes.TaskGroup) string {
	if tskgroup.ObjectMeta.NameSpace != "" && service.ObjectMeta.NameSpace != tskgroup.ObjectMeta.NameSpace {
		blog.V(3).Infof("namespace of service (%s %s) and taskgroup (%s %s) is different",
			service.NameSpace, service.Name, tskgroup.ObjectMeta.NameSpace, tskgroup.ID)
		return ""
	}

	key := service.ObjectMeta.NameSpace + "." + service.ObjectMeta.Name
	for ks, vs := range service.Spec.Selector {
		//blog.V(3).Infof("check service %s selector label:%s -> %s", key, ks, vs)
		task := tskgroup.Taskgroup[0]
		if task.Labels == nil {
			return ""
		}
		for kt, vt := range task.Labels {
			blog.V(3).Infof("check task(%s) label(%s:%s) with selector label(%s:%s)", task.Name, kt, vt, ks, vs)
			if ks == kt && vs == vt {
				blog.V(3).Infof("task label match service: task(%s) label(%s:%s) service(%s)", task.Name, kt, vt, key)
				return vt
			}
		}
	}
	return ""
}

func (watch *ExportServiceWatch) getApplicationServiceLabel(service *commtypes.BcsService, app *schedtypes.Application) string {
	if service.ObjectMeta.NameSpace != app.ObjectMeta.NameSpace {
		blog.V(3).Infof("namespace of service (%s %s) and application (%s %s) is different",
			service.NameSpace, service.Name, app.ObjectMeta.NameSpace, app.ID)
		return ""
	}

	key := service.ObjectMeta.NameSpace + "." + service.ObjectMeta.Name
	for ks, vs := range service.Spec.Selector {

		for kt, vt := range app.ObjectMeta.Labels {
			blog.V(3).Infof("check application(%s %s) label(%s:%s) with selector label(%s:%s)", app.RunAs, app.ID, kt, vt, ks, vs)
			if ks == kt && vs == vt {
				blog.V(3).Infof("application label match service: application(%s %s) label(%s:%s) service(%s)", app.RunAs, app.ID, kt, vt, key)
				return vt
			}
		}
	}
	return ""
}

func (watch *ExportServiceWatch) addEpBackend(ep *lbtypes.ExportPort, backend *lbtypes.Backend) bool {
	if backend.TargetIP == "" || backend.TargetPort <= 0 || backend.Label[0] == "" {
		blog.Error("ExportServiceWatch add backend, backend data not correct: %+v", backend)
		return false
	}
	for _, currBackend := range ep.Backends {
		if backend.TargetIP == currBackend.TargetIP && backend.TargetPort == currBackend.TargetPort {
			blog.V(3).Infof("ExportServiceWatch add backend: %+v, already in, do nothing", backend)
			return false
		}
	}

	blog.V(3).Infof("ExportServiceWatch: exportPort(%s:%d %s) real add backend: %+v ", ep.BCSVHost, ep.ServicePort, ep.Protocol, backend)
	//blog.Info("ExportServiceWatch: exportPort(%s:%d %s) before add backend: %+v ", ep.BCSVHost,ep.ServicePort, ep.Protocol, ep)
	ep.Backends = append(ep.Backends, *backend)
	//blog.Info("ExportServiceWatch: exportPort(%s:%d %s) after add backend: %+v ", ep.BCSVHost,ep.ServicePort, ep.Protocol, ep)
	return true
}

func (watch *ExportServiceWatch) deleteEpBackend(ep *lbtypes.ExportPort, backend *lbtypes.Backend) bool {
	delete := -1
	for index, currBackend := range ep.Backends {
		if backend.TargetIP == currBackend.TargetIP && backend.TargetPort == currBackend.TargetPort {
			delete = index
			break
		}
	}
	if delete == -1 {
		return false
	}

	blog.V(3).Infof("ExportServiceWatch: exportPort(%s:%d %s) real delete backend: %+v ", ep.BCSVHost, ep.ServicePort, ep.Protocol, backend)
	//blog.Info("ExportServiceWatch: exportPort(%s:%d %s) before delete backend: %+v ", ep.BCSVHost,ep.ServicePort, ep.Protocol, ep)
	ep.Backends = append(ep.Backends[:delete], ep.Backends[delete+1:]...)
	//blog.Info("ExportServiceWatch: exportPort(%s:%d %s) after delete backend: %+v ", ep.BCSVHost,ep.ServicePort, ep.Protocol, ep)
	return true
}

func (watch *ExportServiceWatch) addTaskGroup(tskgroup *schedtypes.TaskGroup) {
	blog.Info("ExportServiceWatch receive taskgroup add event, %s: %s", tskgroup.ID, tskgroup.Status)
	if tskgroup.Taskgroup == nil || len(tskgroup.Taskgroup) == 0 {
		blog.Error("ExportServiceWatch receive taskgroup add event, but TaskGroup %s has no Task Info", tskgroup.ID)
		return
	}

	if tskgroup.Status != schedtypes.TASKGROUP_STATUS_RUNNING && tskgroup.Status != schedtypes.TASKGROUP_STATUS_LOST {
		blog.V(3).Infof("ExportServiceWatch receive taskgroup add event, TaskGroup %s status %s, do nothing ", tskgroup.ID, tskgroup.Status)
		return
	}

	keyList := watch.esInfoCache.ListKeys()
	for _, key := range keyList {
		cacheData, exist, err := watch.esInfoCache.GetByKey(key)
		if err != nil {
			blog.Error("esInfo %s in cache keylist, but get return err:%s", err.Error())
			continue
		}
		if exist == false {
			blog.Error("esInfo %s in cache keylist, but get return not exist", key)
			continue
		}
		esInfo, ok := cacheData.(*ExportServiceInfo)
		if !ok {
			blog.Error("convert cachedata to ExportServiceInfo fail, key(%s)", key)
			continue
		}

		// check matching of selector and task label
		label := watch.getTaskGroupServiceLabel(esInfo.bcsService, tskgroup)
		if label == "" {
			continue
		}
		blog.V(3).Infof("ExportServiceWatch: %s, match task label(%s:%s) ", key, tskgroup.ID, label)

		//build backend info
		for index, oneEsPort := range esInfo.exportService.ServicePort {
			for _, oneTask := range tskgroup.Taskgroup {
				for _, onePort := range oneTask.PortMappings {
					if oneEsPort.Name == onePort.Name {
						//match a port
						backend := new(lbtypes.Backend)
						backend.Label = append(backend.Label, label)
						// get IP info from executer reported task.StatusData
						if len(oneTask.StatusData) == 0 {
							blog.V(3).Infof("ExportServiceWatch: %s, task %s StatusData is empty, cannot add to backend", key, oneTask.ID)
							continue
						}
						bcsInfo := new(container.BcsContainerInfo)
						blog.V(3).Infof("ExportServiceWatch: %s, task %s add, StatusData: %s", key, oneTask.ID, oneTask.StatusData)
						if err = json.Unmarshal([]byte(oneTask.StatusData), bcsInfo); err != nil {
							blog.V(3).Infof("ExportServiceWatch: %s, task %s StatusData unmarshal err: %s, cannot add to backend",
								key, oneTask.ID, err.Error())
							continue
						}
						if strings.ToLower(oneTask.Network) == "host" {
							backend.TargetIP = bcsInfo.NodeAddress
							backend.TargetPort = int(onePort.ContainerPort)
						} else if strings.ToLower(oneTask.Network) == "bridge" {
							if onePort.HostPort > 0 {
								backend.TargetIP = bcsInfo.NodeAddress
								backend.TargetPort = int(onePort.HostPort)
							} else {
								backend.TargetIP = bcsInfo.IPAddress
								backend.TargetPort = int(onePort.ContainerPort)
							}
						} else if strings.ToLower(oneTask.NetworkType) == "cnm" {
							if onePort.HostPort > 0 {
								backend.TargetIP = bcsInfo.NodeAddress
								backend.TargetPort = int(onePort.HostPort)
							} else {
								backend.TargetIP = bcsInfo.IPAddress
								backend.TargetPort = int(onePort.ContainerPort)
							}
							//container cni network, docker run --net=none
						} else {
							backend.TargetIP = bcsInfo.IPAddress
							backend.TargetPort = int(onePort.ContainerPort)
						}

						changed := watch.addEpBackend(&oneEsPort, backend)
						if changed == true {
							// value copy
							esInfo.exportService.ServicePort[index] = oneEsPort
							watch.UpdateEvent(esInfo.exportService)
							//blog.Info("ExportServiceWatch: exportPort(%s:%d %s) after add backend: %+v ",
							//		oneEsPort.BCSVHost,oneEsPort.ServicePort, oneEsPort.Protocol, oneEsPort)
							blog.V(3).Infof("ExportServiceWatch after add backend: %+v ", esInfo.exportService)
							blog.Info("service %s add backend for:%s", key, tskgroup.ID)
						}
					}
				}
			}
		}
	}

	return
}

func (watch *ExportServiceWatch) updateTaskGroup(tskgroup *schedtypes.TaskGroup) {
	blog.V(3).Infof("ExportServiceWatch receive taskgroup update event, %s: %s", tskgroup.ID, tskgroup.Status)
	if tskgroup.Taskgroup == nil || len(tskgroup.Taskgroup) == 0 {
		blog.Error("ExportServiceWatch receive taskgroup update event, but TaskGroup %s has no Task Info", tskgroup.ID)
		return
	}

	keyList := watch.esInfoCache.ListKeys()
	for _, key := range keyList {
		cacheData, exist, err := watch.esInfoCache.GetByKey(key)
		if err != nil {
			blog.Error("esInfo %s in cache keylist, but get return err:%s", err.Error())
			continue
		}
		if exist == false {
			blog.Error("esInfo %s in cache keylist, but get return not exist", key)
			continue
		}
		esInfo, ok := cacheData.(*ExportServiceInfo)
		if !ok {
			blog.Error("convert cachedata to ExportServiceInfo fail, key(%s)", key)
			continue
		}

		// check matching of selector and task label
		label := watch.getTaskGroupServiceLabel(esInfo.bcsService, tskgroup)
		if label == "" {
			continue
		}
		blog.V(3).Infof("ExportServiceWatch: %s, match task label(%s: %s) ", key, tskgroup.ID, label)
		//build backend info
		for index, oneEsPort := range esInfo.exportService.ServicePort {
			blog.V(3).Infof("export service: %s, name(%s) ", key, oneEsPort.Name)
			for _, oneTask := range tskgroup.Taskgroup {
				for _, onePort := range oneTask.PortMappings {
					blog.V(3).Infof("task: %s, portname(%s) ", oneTask.Name, onePort.Name)
					if oneEsPort.Name == onePort.Name {
						//match a port
						backend := new(lbtypes.Backend)
						backend.Label = append(backend.Label, label)
						// get IP info from executer reported task.StatusData
						if len(oneTask.StatusData) == 0 {
							blog.V(3).Infof("ExportServiceWatch: %s, task %s StatusData is empty, cannot add or delete backend", key, oneTask.ID)
							continue
						}
						bcsInfo := new(container.BcsContainerInfo)
						blog.V(3).Infof("ExportServiceWatch: %s, task %s update, StatusData: %s", key, oneTask.ID, oneTask.StatusData)
						if err = json.Unmarshal([]byte(oneTask.StatusData), bcsInfo); err != nil {
							blog.V(3).Infof("ExportServiceWatch: %s, task %s StatusData unmarshal err: %s, cannot add or delete backend",
								key, oneTask.ID, err.Error())
							continue
						}
						if strings.ToLower(oneTask.Network) == "host" {
							backend.TargetIP = bcsInfo.NodeAddress
							backend.TargetPort = int(onePort.ContainerPort)
						} else if strings.ToLower(oneTask.Network) == "bridge" {
							if onePort.HostPort > 0 {
								backend.TargetIP = bcsInfo.NodeAddress
								backend.TargetPort = int(onePort.HostPort)
							} else {
								backend.TargetIP = bcsInfo.IPAddress
								backend.TargetPort = int(onePort.ContainerPort)
							}
						} else if strings.ToLower(oneTask.NetworkType) == "cnm" {
							if onePort.HostPort > 0 {
								backend.TargetIP = bcsInfo.NodeAddress
								backend.TargetPort = int(onePort.HostPort)
							} else {
								backend.TargetIP = bcsInfo.IPAddress
								backend.TargetPort = int(onePort.ContainerPort)
							}
							//container cni network, docker run --net=none
						} else {
							backend.TargetIP = bcsInfo.IPAddress
							backend.TargetPort = int(onePort.ContainerPort)
						}

						var changed bool
						if tskgroup.Status == schedtypes.TASKGROUP_STATUS_RUNNING || tskgroup.Status == schedtypes.TASKGROUP_STATUS_LOST {
							changed = watch.addEpBackend(&oneEsPort, backend)
						} else {
							changed = watch.deleteEpBackend(&oneEsPort, backend)
						}
						if changed == true {
							esInfo.exportService.ServicePort[index] = oneEsPort
							watch.UpdateEvent(esInfo.exportService)
							//blog.Info("ExportServiceWatch: exportPort(%s:%d %s) after add backend: %+v ",
							//		oneEsPort.BCSVHost,oneEsPort.ServicePort, oneEsPort.Protocol, oneEsPort)
							blog.V(3).Infof("ExportServiceWatch after add or delete backend: %+v ", esInfo.exportService)
							blog.Info("service %s add or delete backend for:%s", key, tskgroup.ID)
						}
					}
				}
			}
		}
	}

	return
}

func (watch *ExportServiceWatch) deleteTaskGroup(tskgroup *schedtypes.TaskGroup) {
	blog.Info("ExportServiceWatch receive taskgroup delete event, %s: %s", tskgroup.ID, tskgroup.Status)

	if tskgroup.Taskgroup == nil || len(tskgroup.Taskgroup) == 0 {
		blog.Error("ExportServiceWatch receive taskgroup delete event, but TaskGroup %s has no Task Info", tskgroup.ID)
		return
	}

	keyList := watch.esInfoCache.ListKeys()
	for _, key := range keyList {
		cacheData, exist, err := watch.esInfoCache.GetByKey(key)
		if err != nil {
			blog.Error("esInfo %s in cache keylist, but get return err:%s", key, err.Error())
			continue
		}
		if exist == false {
			blog.Error("esInfo %s in cache keylist, but get return not exist", key)
			continue
		}
		esInfo, ok := cacheData.(*ExportServiceInfo)
		if !ok {
			blog.Error("convert cachedata to ExportServiceInfo fail, key(%s)", key)
			continue
		}
		// check matching of selector and task label
		label := watch.getTaskGroupServiceLabel(esInfo.bcsService, tskgroup)
		if label == "" {
			continue
		}

		blog.V(3).Infof("ExportServiceWatch: %s, match task label(%s: %s) ", key, tskgroup.ID, label)

		//delete backend
		for index, oneEsPort := range esInfo.exportService.ServicePort {
			for _, oneTask := range tskgroup.Taskgroup {
				for _, onePort := range oneTask.PortMappings {
					if oneEsPort.Name == onePort.Name {
						//match a port
						backend := new(lbtypes.Backend)
						backend.Label = append(backend.Label, label)
						// get IP info from executer reported task.StatusData
						if len(oneTask.StatusData) == 0 {
							blog.V(3).Infof("ExportServiceWatch: %s, task %s StatusData is empty, cannot add or delete backend", key, oneTask.ID)
							continue
						}

						blog.V(3).Infof("ExportServiceWatch: %s, task %s delete, StatusData: %s", key, oneTask.ID, oneTask.StatusData)
						bcsInfo := new(container.BcsContainerInfo)
						if err = json.Unmarshal([]byte(oneTask.StatusData), bcsInfo); err != nil {
							blog.V(3).Infof("ExportServiceWatch: %s, task %s StatusData unmarshal err: %s, cannot add or delete backend",
								key, oneTask.ID, err.Error())
							continue
						}
						if strings.ToLower(oneTask.Network) == "host" {
							backend.TargetIP = bcsInfo.NodeAddress
							backend.TargetPort = int(onePort.ContainerPort)
						} else if strings.ToLower(oneTask.Network) == "bridge" {
							if onePort.HostPort > 0 {
								backend.TargetIP = bcsInfo.NodeAddress
								backend.TargetPort = int(onePort.HostPort)
							} else {
								backend.TargetIP = bcsInfo.IPAddress
								backend.TargetPort = int(onePort.ContainerPort)
							}
						} else if strings.ToLower(oneTask.NetworkType) == "cnm" {
							if onePort.HostPort > 0 {
								backend.TargetIP = bcsInfo.NodeAddress
								backend.TargetPort = int(onePort.HostPort)
							} else {
								backend.TargetIP = bcsInfo.IPAddress
								backend.TargetPort = int(onePort.ContainerPort)
							}
							//container cni network, docker run --net=none
						} else {
							backend.TargetIP = bcsInfo.IPAddress
							backend.TargetPort = int(onePort.ContainerPort)
						}

						changed := watch.deleteEpBackend(&oneEsPort, backend)
						if changed == true {
							esInfo.exportService.ServicePort[index] = oneEsPort
							watch.UpdateEvent(esInfo.exportService)
							//blog.Info("ExportServiceWatch: exportPort(%s:%d %s) after add backend: %+v ",
							//		oneEsPort.BCSVHost,oneEsPort.ServicePort, oneEsPort.Protocol, oneEsPort)
							blog.V(3).Infof("ExportServiceWatch after delete backend: %+v ", esInfo.exportService)

							blog.Info("service %s delete backend for:%s", key, tskgroup.ID)
						}
					}
				}
			}
		}
	}

	return
}

//stop ask appwatch stop, clean all data
func (watch *ExportServiceWatch) stop() {
	watch.esCache.Clear()
	watch.esInfoCache.Clear()
}

//AddEvent call when data added
func (watch *ExportServiceWatch) AddEvent(obj interface{}) {
	data, ok := obj.(*lbtypes.ExportService)
	if !ok {
		blog.Error("ExportServiceWatch: can not convert object to ExportService in AddEvent, object %v", obj)
		return
	}
	blog.Info("ExportServiceWatch: Add Event for ExportService %s-%s.%s", data.Cluster, data.Namespace, data.ServiceName)

	tmpData := new(lbtypes.ExportService)
	lbtypes.DeepCopy(data, tmpData)

	sync := &types.BcsSyncData{
		DataType: watch.GetExportserviceChannel(tmpData),
		Action:   "Add",
		Item:     tmpData,
	}
	watch.report.ReportData(sync)
}

//DeleteEvent when delete
func (watch *ExportServiceWatch) DeleteEvent(obj interface{}) {
	data, _ := obj.(*lbtypes.ExportService)
	blog.Info("ExportServiceWatch: Delete Event for %s-%s.%s", data.Cluster, data.Namespace, data.ServiceName)

	tmpData := new(lbtypes.ExportService)
	lbtypes.DeepCopy(data, tmpData)
	//report to cluster
	sync := &types.BcsSyncData{
		DataType: watch.GetExportserviceChannel(tmpData),
		Action:   "Delete",
		Item:     tmpData,
	}
	watch.report.ReportData(sync)
}

//UpdateEvent when update
func (watch *ExportServiceWatch) UpdateEvent(obj interface{}) {
	data, _ := obj.(*lbtypes.ExportService)
	blog.V(3).Infof("ExportServiceWatch: Update Event for %s-%s.%s", data.Cluster, data.Namespace, data.ServiceName)

	tmpData := new(lbtypes.ExportService)
	lbtypes.DeepCopy(data, tmpData)

	//report to cluster
	sync := &types.BcsSyncData{
		DataType: watch.GetExportserviceChannel(tmpData),
		Action:   "Update",
		Item:     tmpData,
	}
	watch.report.ReportData(sync)
}

func (watch *ExportServiceWatch) GetExportserviceChannel(exportservice *lbtypes.ExportService) string {

	index := util.GetHashId(exportservice.ServiceName, ExportserviceThreadNum)

	return types.ExportserviceChannelPrefix + strconv.Itoa(index)

}

func (watch *ExportServiceWatch) SyncExportServiceBackends(esInfo *ExportServiceInfo) error {
	basePath := fmt.Sprintf("%s/application/%s", watch.basePath, esInfo.exportService.Namespace)
	blog.Info("sync all taskgroups under(%s)", basePath)

	appList, _, err := watch.client.GetChildrenEx(basePath)
	if err != nil {
		blog.Error("get path(%s) children err: %s", basePath, err.Error())
		return err
	}

	if len(appList) == 0 {
		blog.V(3).Infof("get empty namespace list under path(%s)", basePath)
		return nil
	}

	for _, app := range appList {
		appPath := fmt.Sprintf("%s/%s", basePath, app)

		by, _, err := watch.client.GetEx(appPath)

		var application *schedtypes.Application

		err = json.Unmarshal(by, &application)
		if err != nil {
			blog.Errorf("json.Unmarshal zk path %s data failed, error %s", appPath, err.Error())
			continue
		}

		// check matching of selector and task label
		label := watch.getApplicationServiceLabel(esInfo.bcsService, application)
		if label == "" {
			continue
		}

		blog.V(3).Infof("ExportServiceWatch: exportservice (%s %s) match application label(%s:%s) ",
			esInfo.exportService.Namespace, esInfo.exportService.ServiceName, application.ID, label)

		tgList, _, err := watch.client.GetChildrenEx(appPath)
		if err != nil {
			blog.Error("get path(%s) children err: %s", appPath, err.Error())
			continue
		}

		for _, tg := range tgList {
			tgPath := fmt.Sprintf("%s/%s", appPath, tg)

			by, _, err := watch.client.GetEx(tgPath)
			if err != nil {
				blog.Errorf("get zk path %s error %s", tgPath, err.Error())
				continue
			}

			var taskgroup *schedtypes.TaskGroup

			err = json.Unmarshal(by, &taskgroup)
			if err != nil {
				blog.Errorf("json.Unmarshal zk path %s data failed, error %s", tgPath, err.Error())
				continue
			}

			err = watch.SyncEpTaskgroupBackend(esInfo, taskgroup)
			if err != nil {
				blog.Errorf("ExportServiceWatch: service (%s %s) taskgroup %s GetEpBackend error %s",
					esInfo.bcsService.NameSpace, esInfo.bcsService.Name, taskgroup.ID, err.Error())
			}
		}

	}

	return nil
}

func (watch *ExportServiceWatch) SyncEpTaskgroupBackend(esInfo *ExportServiceInfo, taskgroup *schedtypes.TaskGroup) error {
	if taskgroup.Status != schedtypes.TASKGROUP_STATUS_RUNNING && taskgroup.Status != schedtypes.TASKGROUP_STATUS_LOST {
		blog.V(3).Infof("ExportServiceWatch receive taskgroup add event, TaskGroup %s status %s, do nothing ", taskgroup.ID, taskgroup.Status)
		return nil
	}

	// check matching of selector and task label
	label := watch.getTaskGroupServiceLabel(esInfo.bcsService, taskgroup)
	if label == "" {
		return nil
	}

	blog.V(3).Infof("ExportServiceWatch: exportservice (%s %s) match task label(%s:%s) ",
		esInfo.exportService.Namespace, esInfo.exportService.ServiceName, taskgroup.ID, label)

	//build backend info
	for index, oneEsPort := range esInfo.exportService.ServicePort {
		for _, oneTask := range taskgroup.Taskgroup {
			for _, onePort := range oneTask.PortMappings {
				if oneEsPort.Name == onePort.Name {
					//match a port
					backend := new(lbtypes.Backend)
					backend.Label = append(backend.Label, label)
					// get IP info from executer reported task.StatusData
					if len(oneTask.StatusData) == 0 {
						blog.V(3).Infof("ExportServiceWatch: service (%s %s) task %s StatusData is empty, cannot add to backend",
							esInfo.exportService.Namespace, esInfo.exportService.ServiceName, oneTask.ID)
						continue
					}

					bcsInfo := new(container.BcsContainerInfo)
					blog.V(3).Infof("ExportServiceWatch: service (%s %s) task %s add, StatusData: %s",
						esInfo.exportService.Namespace, esInfo.exportService.ServiceName, oneTask.ID, oneTask.StatusData)

					if err := json.Unmarshal([]byte(oneTask.StatusData), bcsInfo); err != nil {
						blog.V(3).Infof("ExportServiceWatch: service (%s %s) task %s StatusData unmarshal err: %s, cannot add to backend",
							esInfo.exportService.Namespace, esInfo.exportService.ServiceName, oneTask.ID, err.Error())
						continue
					}

					//container docker host network, docker run --net=host
					if strings.ToLower(oneTask.Network) == "host" {
						backend.TargetIP = bcsInfo.NodeAddress
						backend.TargetPort = int(onePort.ContainerPort)
						blog.V(3).Infof("ExportServiceWatch: service (%s %s) backend targetip %s targetport %d",
							esInfo.exportService.Namespace, esInfo.exportService.ServiceName, backend.TargetIP, backend.TargetPort)
						//container docker bridge network, docker run --net=bridge
					} else if strings.ToLower(oneTask.Network) == "bridge" {
						if onePort.HostPort > 0 {
							backend.TargetIP = bcsInfo.NodeAddress
							backend.TargetPort = int(onePort.HostPort)
						} else {
							backend.TargetIP = bcsInfo.IPAddress
							backend.TargetPort = int(onePort.ContainerPort)
						}
						blog.V(3).Infof("ExportServiceWatch: service (%s %s) backend targetip %s targetport %d",
							esInfo.exportService.Namespace, esInfo.exportService.ServiceName, backend.TargetIP, backend.TargetPort)
						//container docker user defined network, docker run --net=mynetwork
					} else if strings.ToLower(oneTask.NetworkType) == "cnm" {
						if onePort.HostPort > 0 {
							backend.TargetIP = bcsInfo.NodeAddress
							backend.TargetPort = int(onePort.HostPort)
						} else {
							backend.TargetIP = bcsInfo.IPAddress
							backend.TargetPort = int(onePort.ContainerPort)
						}
						blog.V(3).Infof("ExportServiceWatch: service (%s %s) backend targetip %s targetport %d",
							esInfo.exportService.Namespace, esInfo.exportService.ServiceName, backend.TargetIP, backend.TargetPort)
						//container cni network, docker run --net=none
					} else {
						backend.TargetIP = bcsInfo.IPAddress
						backend.TargetPort = int(onePort.ContainerPort)
						blog.V(3).Infof("ExportServiceWatch: service (%s %s) backend targetip %s targetport %d",
							esInfo.exportService.Namespace, esInfo.exportService.ServiceName, backend.TargetIP, backend.TargetPort)
					}

					changed := watch.addEpBackend(&oneEsPort, backend)
					if changed {
						esInfo.exportService.ServicePort[index] = oneEsPort
					}
				}
			}
		}
	}

	return nil
}
