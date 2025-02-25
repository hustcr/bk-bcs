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

package list

import (
	"fmt"

	"bk-bcs/bcs-services/bcs-client/cmd/utils"

	"github.com/urfave/cli"
)

func NewListCommand() cli.Command {
	return cli.Command{
		Name:  "list",
		Usage: "list brief information of application, taskgroup, agent, cluster etc.",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "type, t",
				Usage: "List type, ns/app/process/taskgroup/service/configmap/secret/deployment/endpoint/agent",
			},
			cli.StringFlag{
				Name:  "clusterid",
				Usage: "Cluster ID",
			},
			cli.StringFlag{
				Name:  "namespace, ns",
				Usage: "Namespace",
				Value: "",
			},
			cli.StringFlag{
				Name:  "ip",
				Usage: "The ip of taskgroup. Split by ,",
			},
		},
		Action: func(c *cli.Context) error {
			if err := list(utils.NewClientContext(c)); err != nil {
				return err
			}
			return nil
		},
	}
}

func list(c *utils.ClientContext) error {
	if err := c.MustSpecified(utils.OptionType); err != nil {
		return err
	}

	resourceType := c.String(utils.OptionType)

	switch resourceType {
	case "ns", "namespace":
		return listNamespace(c)
	case "app", "application":
		return listApplication(c)
	case "process":
		return listProcess(c)
	case "tg", "taskgroup":
		return listTaskGroup(c)
	case "configmap":
		return listConfigMap(c)
	case "secret":
		return listSecret(c)
	case "service":
		return listService(c)
	case "deploy", "deployment":
		return listDeployment(c)
	case "endpoint":
		return listEndpoint(c)
	case "agent":
		return listAgent(c)
	default:
		return fmt.Errorf("invalid type: %s", resourceType)
	}
}
