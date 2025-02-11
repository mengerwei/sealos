// Copyright © 2021 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package aliyun

import (
	"fmt"
	"strings"

	"github.com/fanux/sealos/pkg/logger"

	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/fanux/sealos/pkg/utils"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/pkg/errors"

	"github.com/aliyun/alibaba-cloud-sdk-go/services/ecs"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/vpc"
	"github.com/fanux/sealos/pkg/types/v1beta1"
)

type ActionName string

const (
	CreateVPC           ActionName = "CreateVPC"
	CreateVSwitch       ActionName = "CreateVSwitch"
	CreateSecurityGroup ActionName = "CreateSecurityGroup"
	ReconcileInstance   ActionName = "ReconcileInstance"
	BindEIP             ActionName = "BindEIP"
	ReleaseEIP          ActionName = "ReleaseEIP"
	ClearInstances      ActionName = "ClearInstances"
	DeleteVSwitch       ActionName = "DeleteVSwitch"
	DeleteSecurityGroup ActionName = "DeleteSecurityGroup"
	DeleteVPC           ActionName = "DeleteVPC"
	GetZoneID           ActionName = "GetAvailableZoneID"
)

type AliProvider struct {
	EcsClient ecs.Client
	VpcClient vpc.Client
	Infra     *v1beta1.Infra
}

type AliFunc func() error

func (a *AliProvider) ReconcileResource(resourceKey ResourceName, action AliFunc) error {
	if resourceKey.Value(a.Infra.Status) != "" {
		logger.Debug("using resource status value %s: %s", resourceKey, resourceKey.Value(a.Infra.Status))
		return nil
	}
	if err := action(); err != nil {
		logger.Error("reconcile resource %s failed err: %s", resourceKey, err)
		return err
	}
	if resourceKey.Value(a.Infra.Status) != "" {
		logger.Info("create resource success %s: %s", resourceKey, resourceKey.Value(a.Infra.Status))
	}
	return nil
}

func (a *AliProvider) DeleteResource(resourceKey ResourceName, action AliFunc) {
	val := resourceKey.Value(a.Infra.Status)
	if val == "" {
		logger.Warn("delete resource not exists %s", resourceKey)
		return
	}
	if err := action(); err != nil {
		logger.Error("delete resource %s failed err: %s", resourceKey, err)
	} else {
		logger.Info("delete resource Success %s: %s", resourceKey, val)
	}
}

var RecocileFuncMap = map[ActionName]func(provider *AliProvider) error{
	CreateVPC: func(aliProvider *AliProvider) error {
		return aliProvider.ReconcileResource(VpcID, aliProvider.CreateVPC)
	},

	CreateVSwitch: func(aliProvider *AliProvider) error {
		return aliProvider.ReconcileResource(VSwitchID, aliProvider.CreateVSwitch)
	},
	CreateSecurityGroup: func(aliProvider *AliProvider) error {
		return aliProvider.ReconcileResource(SecurityGroupID, aliProvider.CreateSecurityGroup)
	},
	ReconcileInstance: func(aliProvider *AliProvider) error {
		var errorMsg []string
		current := sets.NewString()
		spec := sets.NewString()
		for _, h := range aliProvider.Infra.Status.Hosts {
			current.Insert(strings.Join(h.Roles, ","))
		}
		for _, h := range aliProvider.Infra.Spec.Hosts {
			spec.Insert(strings.Join(h.Roles, ","))
			host := &h
			statusIndex := aliProvider.Infra.Status.FindHostsByRoles(h.Roles)
			if statusIndex < 0 {
				errorMsg = append(errorMsg, fmt.Sprintf("infra status not fount in role tag: %v", h.Roles))
				continue
			}
			status := &aliProvider.Infra.Status.Hosts[statusIndex]
			err := aliProvider.ReconcileInstances(host, status)
			if err != nil {
				errorMsg = append(errorMsg, err.Error())
				status.Ready = false
			} else {
				status.Ready = true
			}
		}
		deleteData := current.Difference(spec)
		var instanceIDs []string
		finalStatus := aliProvider.Infra.Status.Hosts
		for _, roles := range deleteData.List() {
			statusIndex := aliProvider.Infra.Status.FindHostsByRolesString(roles)
			ids := aliProvider.Infra.Status.Hosts[statusIndex].IDs
			instanceIDs = append(instanceIDs, ids)
			finalStatus = append(finalStatus[:statusIndex], finalStatus[statusIndex+1:]...)
		}
		if len(instanceIDs) != 0 {
			ShouldBeDeleteInstancesIDs.SetValue(aliProvider.Infra.Status, strings.Join(instanceIDs, ","))
			aliProvider.DeleteResource(ShouldBeDeleteInstancesIDs, aliProvider.DeleteInstances)
			aliProvider.Infra.Status.Hosts = finalStatus
		}

		if len(errorMsg) == 0 {
			return nil
		}
		return errors.New(strings.Join(errorMsg, " && "))
	},
	GetZoneID: func(aliProvider *AliProvider) error {
		return aliProvider.ReconcileResource(ZoneID, aliProvider.GetAvailableZoneID)
	},
	BindEIP: func(aliProvider *AliProvider) error {
		return aliProvider.ReconcileResource(EipID, aliProvider.BindEipForMaster0)
	},
}

var DeleteFuncMap = map[ActionName]func(provider *AliProvider){
	ReleaseEIP: func(aliProvider *AliProvider) {
		aliProvider.DeleteResource(EipID, aliProvider.ReleaseEipAddress)
	},
	ClearInstances: func(aliProvider *AliProvider) {
		var instanceIDs []string
		for _, h := range aliProvider.Infra.Status.Hosts {
			instances, err := aliProvider.GetInstancesInfo(h.ToHost(), JustGetInstanceInfo)
			if err != nil {
				logger.Error("get %s instanceInfo failed %v", strings.Join(h.Roles, ","), err)
			}
			for _, instance := range instances {
				instanceIDs = append(instanceIDs, instance.InstanceID)
			}
		}

		if len(instanceIDs) != 0 {
			ShouldBeDeleteInstancesIDs.SetValue(aliProvider.Infra.Status, strings.Join(instanceIDs, ","))
		}
		aliProvider.DeleteResource(ShouldBeDeleteInstancesIDs, aliProvider.DeleteInstances)
	},
	DeleteVSwitch: func(aliProvider *AliProvider) {
		aliProvider.DeleteResource(VSwitchID, aliProvider.DeleteVSwitch)
	},
	DeleteSecurityGroup: func(aliProvider *AliProvider) {
		aliProvider.DeleteResource(SecurityGroupID, aliProvider.DeleteSecurityGroup)
	},
	DeleteVPC: func(aliProvider *AliProvider) {
		aliProvider.DeleteResource(VpcID, aliProvider.DeleteVPC)
	},
}

func (a *AliProvider) NewClient() error {
	regionID := a.Infra.Spec.Cluster.RegionIDs[utils.Rand(len(a.Infra.Spec.Cluster.RegionIDs))]
	a.Infra.Status.Cluster.RegionID = regionID
	logger.Info("using regionID is %s", regionID)
	ecsClient, err := ecs.NewClientWithAccessKey(a.Infra.Status.Cluster.RegionID, a.Infra.Spec.Credential.AccessKey, a.Infra.Spec.Credential.AccessSecret)
	if err != nil {
		return err
	}
	vpcClient, err := vpc.NewClientWithAccessKey(a.Infra.Status.Cluster.RegionID, a.Infra.Spec.Credential.AccessKey, a.Infra.Spec.Credential.AccessSecret)
	if err != nil {
		return err
	}
	a.EcsClient = *ecsClient
	a.VpcClient = *vpcClient
	return nil
}

func (a *AliProvider) ClearCluster() {
	todolist := []ActionName{
		ReleaseEIP,
		ClearInstances,
		DeleteVSwitch,
		DeleteSecurityGroup,
		DeleteVPC,
	}
	for _, name := range todolist {
		DeleteFuncMap[name](a)
	}
}

func (a *AliProvider) Reconcile() error {
	if a.Infra.DeletionTimestamp != nil {
		logger.Info("DeletionTimestamp not nil Clear Infra")
		a.ClearCluster()
		return nil
	}
	todolist := []ActionName{
		GetZoneID,
		CreateVPC,
		CreateVSwitch,
		CreateSecurityGroup,
		ReconcileInstance,
		BindEIP,
	}

	for _, actionname := range todolist {
		err := RecocileFuncMap[actionname](a)
		if err != nil {
			logger.Warn("actionName: %s,err: %v ,skip it", actionname, err)
			//return err
		}
	}

	return nil
}

func (a *AliProvider) Apply() error {
	return a.Reconcile()
}

func DefaultInfra(infra *v1beta1.Infra) error {
	//https://help.aliyun.com/document_detail/63440.htm?spm=a2c4g.11186623.0.0.f5952752gkxpB7#t9856.html
	if infra.Spec.Cluster.Metadata.IsSeize {
		infra.Status.Cluster.SpotStrategy = "SpotAsPriceGo"
	} else {
		infra.Status.Cluster.SpotStrategy = "NoSpot"
	}
	return nil
}

func DefaultValidate(infra *v1beta1.Infra) field.ErrorList {
	allErrors := field.ErrorList{}
	return allErrors
}
