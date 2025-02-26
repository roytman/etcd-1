// Copyright 2015 The etcd Authors
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

package integration

import (
	"context"
	"fmt"
	"io/ioutil"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.etcd.io/etcd/client/v2"
	"go.etcd.io/etcd/tests/v3/framework/integration"
)

func TestPauseMember(t *testing.T) {
	integration.BeforeTest(t)

	c := integration.NewCluster(t, 5)
	c.Launch(t)
	defer c.Terminate(t)

	for i := 0; i < 5; i++ {
		c.Members[i].Pause()
		membs := append([]*integration.Member{}, c.Members[:i]...)
		membs = append(membs, c.Members[i+1:]...)
		c.WaitMembersForLeader(t, membs)
		clusterMustProgress(t, membs)
		c.Members[i].Resume()
	}
	c.WaitMembersForLeader(t, c.Members)
	clusterMustProgress(t, c.Members)
}

func TestRestartMember(t *testing.T) {
	integration.BeforeTest(t)
	c := integration.NewClusterFromConfig(t, &integration.ClusterConfig{Size: 3, UseBridge: true})
	c.Launch(t)
	defer c.Terminate(t)

	for i := 0; i < 3; i++ {
		c.Members[i].Stop(t)
		membs := append([]*integration.Member{}, c.Members[:i]...)
		membs = append(membs, c.Members[i+1:]...)
		c.WaitMembersForLeader(t, membs)
		clusterMustProgress(t, membs)
		err := c.Members[i].Restart(t)
		if err != nil {
			t.Fatal(err)
		}
	}
	c.WaitMembersForLeader(t, c.Members)
	clusterMustProgress(t, c.Members)
}

func TestLaunchDuplicateMemberShouldFail(t *testing.T) {
	integration.BeforeTest(t)
	size := 3
	c := integration.NewCluster(t, size)
	m := c.Members[0].Clone(t)
	var err error
	m.DataDir, err = ioutil.TempDir(t.TempDir(), "etcd")
	if err != nil {
		t.Fatal(err)
	}
	c.Launch(t)
	defer c.Terminate(t)

	if err := m.Launch(); err == nil {
		t.Errorf("unexpect successful launch")
	} else {
		t.Logf("launch failed as expected: %v", err)
		assert.Contains(t, err.Error(), "has already been bootstrapped")
	}
}

func TestSnapshotAndRestartMember(t *testing.T) {
	integration.BeforeTest(t)
	m := integration.MustNewMember(t, integration.MemberConfig{Name: "snapAndRestartTest", UseBridge: true})
	m.SnapshotCount = 100
	m.Launch()
	defer m.Terminate(t)
	m.WaitOK(t)

	resps := make([]*client.Response, 120)
	var err error
	for i := 0; i < 120; i++ {
		cc := integration.MustNewHTTPClient(t, []string{m.URL()}, nil)
		kapi := client.NewKeysAPI(cc)
		ctx, cancel := context.WithTimeout(context.Background(), integration.RequestTimeout)
		key := fmt.Sprintf("foo%d", i)
		resps[i], err = kapi.Create(ctx, "/"+key, "bar")
		if err != nil {
			t.Fatalf("#%d: create on %s error: %v", i, m.URL(), err)
		}
		cancel()
	}
	m.Stop(t)
	m.Restart(t)

	m.WaitOK(t)
	for i := 0; i < 120; i++ {
		cc := integration.MustNewHTTPClient(t, []string{m.URL()}, nil)
		kapi := client.NewKeysAPI(cc)
		ctx, cancel := context.WithTimeout(context.Background(), integration.RequestTimeout)
		key := fmt.Sprintf("foo%d", i)
		resp, err := kapi.Get(ctx, "/"+key, nil)
		if err != nil {
			t.Fatalf("#%d: get on %s error: %v", i, m.URL(), err)
		}
		cancel()

		if !reflect.DeepEqual(resp.Node, resps[i].Node) {
			t.Errorf("#%d: node = %v, want %v", i, resp.Node, resps[i].Node)
		}
	}
}
