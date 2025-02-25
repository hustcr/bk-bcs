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

package store

import (
	"bk-bcs/bcs-common/common/blog"
	commtypes "bk-bcs/bcs-common/common/types"
	"bk-bcs/bcs-mesos/bcs-scheduler/src/types"
	"encoding/json"
	"github.com/samuel/go-zookeeper/zk"
)

func getAgentRootPath() string {
	return "/" + bcsRootNode + "/" + agentNode
}

func getAgentSettingRootPath() string {
	return "/" + bcsRootNode + "/" + agentSettingNode
}

func getAgentSchedInfoRootPath() string {
	return "/" + bcsRootNode + "/" + agentSchedInfoNode
}

func (store *managerStore) SaveAgent(agent *types.Agent) error {

	data, err := json.Marshal(agent)
	if err != nil {
		return err
	}

	path := getAgentRootPath() + "/" + agent.Key

	return store.Db.Insert(path, string(data))
}

func (store *managerStore) FetchAgent(Key string) (*types.Agent, error) {

	path := getAgentRootPath() + "/" + Key

	data, err := store.Db.Fetch(path)
	if err != nil {
		return nil, err
	}

	agent := &types.Agent{}
	if err := json.Unmarshal(data, agent); err != nil {
		blog.Error("fail to unmarshal agent(%s). err:%s", string(data), err.Error())
		return nil, err
	}

	return agent, nil
}

func (store *managerStore) ListAgentNodes() ([]string, error) {

	path := getAgentRootPath()

	agentNodes, err := store.Db.List(path)
	if err != nil {
		blog.Error("fail to list agents(%s), err:%s", path, err.Error())
		return nil, err
	}

	return agentNodes, nil
}

func (store *managerStore) DeleteAgent(key string) error {

	path := getAgentRootPath() + "/" + key
	if err := store.Db.Delete(path); err != nil {
		blog.Error("fail to delete agent(%s) err:%s", path, err.Error())
		return err
	}

	return nil
}

func (store *managerStore) SaveAgentSetting(agent *commtypes.BcsClusterAgentSetting) error {

	data, err := json.Marshal(agent)
	if err != nil {
		return err
	}

	path := getAgentSettingRootPath() + "/" + agent.InnerIP

	return store.Db.Insert(path, string(data))
}

func (store *managerStore) FetchAgentSetting(InnerIP string) (*commtypes.BcsClusterAgentSetting, error) {

	path := getAgentSettingRootPath() + "/" + InnerIP

	data, err := store.Db.Fetch(path)

	if err == zk.ErrNoNode {
		blog.V(3).Infof("agentSetting(%s) not exist", path)
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	agent := &commtypes.BcsClusterAgentSetting{}
	if err := json.Unmarshal(data, agent); err != nil {
		blog.Error("fail to unmarshal agentSetting(%s). err:%s", string(data), err.Error())
		return nil, err
	}

	return agent, nil
}

func (store *managerStore) DeleteAgentSetting(InnerIP string) error {

	path := getAgentSettingRootPath() + "/" + InnerIP
	if err := store.Db.Delete(path); err != nil {
		blog.Error("fail to delete agentSetting(%s) err:%s", path, err.Error())
		return err
	}
	return nil
}

func (store *managerStore) ListAgentSettingNodes() ([]string, error) {

	path := getAgentSettingRootPath()

	agentNodes, err := store.Db.List(path)
	if err != nil {
		blog.Error("fail to list agentsettings(%s), err:%s", path, err.Error())
		return nil, err
	}

	return agentNodes, nil
}

func (store *managerStore) SaveAgentSchedInfo(agent *types.AgentSchedInfo) error {

	data, err := json.Marshal(agent)
	if err != nil {
		return err
	}

	path := getAgentSchedInfoRootPath() + "/" + agent.HostName

	return store.Db.Insert(path, string(data))
}

func (store *managerStore) FetchAgentSchedInfo(HostName string) (*types.AgentSchedInfo, error) {

	path := getAgentSchedInfoRootPath() + "/" + HostName

	data, err := store.Db.Fetch(path)

	if err == zk.ErrNoNode {
		blog.V(3).Infof("agentSchedInfo(%s) not exist", path)
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	agent := &types.AgentSchedInfo{}
	if err := json.Unmarshal(data, agent); err != nil {
		blog.Error("fail to unmarshal agentSchedInfo(%s). err:%s", string(data), err.Error())
		return nil, err
	}

	return agent, nil
}

func (store *managerStore) DeleteAgentSchedInfo(HostName string) error {

	path := getAgentSchedInfoRootPath() + "/" + HostName
	if err := store.Db.Delete(path); err != nil {
		blog.Error("fail to delete agentSchedInfo(%s) err:%s", path, err.Error())
		return err
	}
	return nil
}
