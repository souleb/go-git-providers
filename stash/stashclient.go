/*
Copyright 2020 The Flux CD contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package stash

import (
	"context"

	"github.com/go-logr/logr"

	"github.com/fluxcd/go-git-providers/gitprovider"
	"github.com/fluxcd/go-git-providers/httpclient"
)

// stashClientImpl is a wrapper around httpclient.ReqResp, which implements rest API access,
// Pagination is implemented for all List* methods, all returned
// objects are validated, and HTTP errors are handled/wrapped using handleHTTPError.
// This interface is also fakeable, in order to unit-test the client.
type stashClient interface {
	// Client returns the underlying httpclient.ReqResp
	Client() httpclient.Requester

	// GetUser returns the user details for a given user name.
	GetUser(ctx context.Context, user string) (*User, error)

	// Group methods

	// GetGroup is a wrapper for "GET /rest/api/1.0/admin/groups?filter={group}".
	// This function HTTP error wrapping, and validates the server result.
	GetGroup(ctx context.Context, groupID interface{}) (*Group, error)

	// ListGroups is a wrapper for "GET /rest/api/1.0/admin/groups".
	// This function handles pagination, HTTP error wrapping, and validates the server result.
	ListGroups(ctx context.Context) ([]*Group, error)

	// ListGroupMembers is a wrapper for "GET /rest/api/1.0/admin/groups/more-members?context={group}".
	// It retruns the users who are members of a group/project
	// This function handles pagination, HTTP error wrapping, and validates the server result.
	ListGroupMembers(ctx context.Context, groupID interface{}) ([]*User, error)

	// GetGroupMembers is a wrapper for "GET /rest/api/1.0/admin/groups/more-members?context={group}&filter={user}".
	// It returns the user if a member of a group/project or nil if not
	GetGroupMember(ctx context.Context, groupID interface{}, userID interface{}) (*GroupMembers, error)

	// Project methods

	// GetProject is a wrapper for "GET /rest/api/1.0/projects?filter={project}".
	// This function handles HTTP error wrapping, and validates the server result.
	GetProject(ctx context.Context, projectName string) (*Project, error)

	// ListProjects is a wrapper for "GET /rest/api/1.0/projects".
	// This function handles pagination, HTTP error wrapping, and validates the server result.
	ListProjects(ctx context.Context) ([]*Project, error)

	// ListProjectGroups is a wrapper for "GET /rest/api/1.0/projects/permissions/groups".
	// This function handles pagination, HTTP error wrapping, and validates the server result.
	ListProjectGroups(ctx context.Context, projectName string) ([]*ProjectGroupPermission, error)

	// getOwnerID retruns the project slug. If the project name is not found it returns the name with tilde prefix to be used as a user name.
	getOwnerID(ctx context.Context, projectName string) string

	//getLogger gets the logger
	getLogger() logr.Logger
}

// stashClientImpl is a wrapper around httpclient.ReqResp and Client
// Pagination is implemented for all List* methods, all returned
// objects are validated, and HTTP errors are handled/wrapped using handleHTTPError.
type stashClientImpl struct {
	c                  httpclient.Requester
	destructiveActions bool
	log                logr.Logger
}

// stashClientImpl implements stashClient.
var _ stashClient = &stashClientImpl{}

func (c *stashClientImpl) Client() httpclient.Requester {
	return c.c
}

func (c *stashClientImpl) getLogger() logr.Logger {
	return c.log
}

func (c *stashClientImpl) getOwnerID(ctx context.Context, projectName string) string {
	if len(projectName) > 0 && projectName[0] == '~' {
		return projectName
	}
	project, err := c.GetProject(ctx, projectName)
	if err != nil {
		return addTilde(projectName)
	}
	return project.Key
}

func (c *stashClientImpl) GetUser(ctx context.Context, userName string) (*User, error) {
	users := NewStashUsers(c)
	user, err := users.Get(ctx, userName)
	if err != nil {
		return nil, err
	}
	// Validate the API objects
	if err := validateUserAPI(user); err != nil {
		return nil, err
	}

	return user, nil
}

func (c *stashClientImpl) GetGroup(ctx context.Context, groupID interface{}) (*Group, error) {
	groups := NewStashGroups(c)
	group, err := groups.Get(ctx, groupID.(string))
	if err != nil {
		return nil, err
	}
	// Validate the API objects
	if err := validateGroupAPI(group); err != nil {
		return nil, err
	}

	return group, nil
}

func (c *stashClientImpl) ListGroups(ctx context.Context) ([]*Group, error) {
	groups := NewStashGroups(c)
	apiObjs := []*Group{}
	opts := &ListOptions{}
	err := allPages(opts, func() (*Paging, error) {
		// GET /groups
		paging, listErr := groups.List(ctx, opts)
		apiObjs = append(apiObjs, groups.getGroups()...)
		return paging, listErr
	})
	if err != nil {
		return nil, err
	}
	// Validate the API objects
	for _, apiObj := range apiObjs {
		if err := validateGroupAPI(apiObj); err != nil {
			return nil, err
		}
	}
	return apiObjs, nil
}

func (c *stashClientImpl) GetGroupMember(ctx context.Context, groupID interface{}, userID interface{}) (*GroupMembers, error) {
	return nil, gitprovider.ErrNoProviderSupport
}

func (c *stashClientImpl) ListGroupMembers(ctx context.Context, groupID interface{}) ([]*User, error) {
	groupMembers := NewStashGroupMembers(c)
	opts := &ListOptions{}
	apiObjs := []*User{}
	err := allPages(opts, func() (*Paging, error) {
		// GET /groups
		paging, listErr := groupMembers.List(ctx, groupID.(string), opts)
		apiObjs = append(apiObjs, groupMembers.getGroupMembers()...)
		return paging, listErr
	})
	if err != nil {
		return nil, err
	}
	// Validate the API objects
	for _, apiObj := range apiObjs {
		if err := validateUserAPI(apiObj); err != nil {
			return nil, err
		}
	}
	return apiObjs, nil
}

func (c *stashClientImpl) GetProject(ctx context.Context, projectName string) (*Project, error) {
	projects := NewStashProjects(c)
	project, err := projects.Get(ctx, projectName)
	if err != nil {
		return nil, err
	}
	// Validate the API objects
	if err := validateProjectAPI(project); err != nil {
		return nil, err
	}

	return project, nil
}

func (c *stashClientImpl) ListProjects(ctx context.Context) ([]*Project, error) {
	projects := NewStashProjects(c)
	apiObjs := []*Project{}
	opts := &ListOptions{}
	err := allPages(opts, func() (*Paging, error) {
		// GET /projects
		paging, listErr := projects.List(ctx, opts)
		apiObjs = append(apiObjs, projects.getProjects()...)
		return paging, listErr
	})
	if err != nil {
		return nil, err
	}
	// Validate the API objects
	for _, apiObj := range apiObjs {
		if err := validateProjectAPI(apiObj); err != nil {
			return nil, err
		}
	}
	return apiObjs, nil
}

func (c *stashClientImpl) ListProjectGroups(ctx context.Context, projectName string) ([]*ProjectGroupPermission, error) {
	projectGroups := NewStashProjectGroups(c, c.getOwnerID(ctx, projectName))
	apiObjs := []*ProjectGroupPermission{}
	opts := &ListOptions{}
	err := allPages(opts, func() (*Paging, error) {
		// GET /projects
		paging, listErr := projectGroups.List(ctx, opts)
		apiObjs = append(apiObjs, projectGroups.getGroups()...)
		return paging, listErr
	})
	if err != nil {
		return nil, err
	}
	// Validate the API objects
	for _, apiObj := range apiObjs {
		if err := validateProjectGroupPermissionAPI(apiObj); err != nil {
			return nil, err
		}
	}
	return apiObjs, nil
}
