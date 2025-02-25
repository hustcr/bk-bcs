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

package v4http

import (
	"bk-bcs/bcs-common/common"
	"bk-bcs/bcs-common/common/blog"
	bhttp "bk-bcs/bcs-common/common/http"
	bcstype "bk-bcs/bcs-common/common/types"
	//"bk-bcs/bcs-mesos/bcs-scheduler/src/types"
	"encoding/json"
	//"github.com/golang/protobuf/proto"
	//"strconv"
)

func (s *Scheduler) UpdateApplication(body []byte, instances, args string) (string, error) {
	blog.Info("update application. param(%s), instances(%s), args(%s)", string(body), instances, args)
	var param bcstype.ReplicaController
	//encoding param by json
	if err := json.Unmarshal(body, &param); err != nil {
		blog.Error("parse parameters failed. param(%s), err(%s)", string(body), err.Error())
		err = bhttp.InternalError(common.BcsErrCommJsonDecode, common.BcsErrCommJsonDecodeStr)
		return err.Error(), err
	}

	// bcs-mesos-scheduler version
	version, err := s.newVersionWithParam(&param)
	if err != nil {
		return err.Error(), err
	}

	version.RawJson = &param

	// post version to bcs-mesos-scheduler, /v1/apps
	data, err := json.Marshal(version)
	if err != nil {
		blog.Error("marshal parameter version by json failed. err:%s", err.Error())
		err = bhttp.InternalError(common.BcsErrCommJsonEncode, common.BcsErrCommJsonEncodeStr+"encode version by json")
		return err.Error(), err
	}

	if s.GetHost() == "" {
		blog.Error("no scheduler is connected by driver")
		err := bhttp.InternalError(common.BcsErrCommHttpDo, common.BcsErrCommHttpDoStr+"scheduler not exist")
		return err.Error(), err
	}

	url := s.GetHost() + "/v1/apps/" + version.RunAs + "/" + version.ID + "/" + "update?instances=" + instances + "&args=" + args
	blog.Info("post a request to url(%s), request:%s", url, string(data))

	//reply, err := bhttp.Request(url, "POST", nil, strings.NewReader(string(data)))
	reply, err := s.client.POST(url, nil, data)
	if err != nil {
		blog.Error("post request to url(%s) failed! err(%s)", url, err.Error())
		err = bhttp.InternalError(common.BcsErrCommHttpDo, common.BcsErrCommHttpDoStr+err.Error())
		return err.Error(), err
	}

	return string(reply), nil
}
