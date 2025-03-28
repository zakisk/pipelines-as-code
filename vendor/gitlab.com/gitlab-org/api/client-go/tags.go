//
// Copyright 2021, Sander van Harmelen
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
//

package gitlab

import (
	"fmt"
	"net/http"
	"net/url"
)

type (
	TagsServiceInterface interface {
		ListTags(pid interface{}, opt *ListTagsOptions, options ...RequestOptionFunc) ([]*Tag, *Response, error)
		GetTag(pid interface{}, tag string, options ...RequestOptionFunc) (*Tag, *Response, error)
		CreateTag(pid interface{}, opt *CreateTagOptions, options ...RequestOptionFunc) (*Tag, *Response, error)
		DeleteTag(pid interface{}, tag string, options ...RequestOptionFunc) (*Response, error)
		CreateReleaseNote(pid interface{}, tag string, opt *CreateReleaseNoteOptions, options ...RequestOptionFunc) (*ReleaseNote, *Response, error)
		UpdateReleaseNote(pid interface{}, tag string, opt *UpdateReleaseNoteOptions, options ...RequestOptionFunc) (*ReleaseNote, *Response, error)
	}

	// TagsService handles communication with the tags related methods
	// of the GitLab API.
	//
	// GitLab API docs: https://docs.gitlab.com/ee/api/tags.html
	TagsService struct {
		client *Client
	}
)

var _ TagsServiceInterface = (*TagsService)(nil)

// Tag represents a GitLab tag.
//
// GitLab API docs: https://docs.gitlab.com/ee/api/tags.html
type Tag struct {
	Commit    *Commit      `json:"commit"`
	Release   *ReleaseNote `json:"release"`
	Name      string       `json:"name"`
	Message   string       `json:"message"`
	Protected bool         `json:"protected"`
	Target    string       `json:"target"`
}

// ReleaseNote represents a GitLab version release.
//
// GitLab API docs: https://docs.gitlab.com/ee/api/tags.html
type ReleaseNote struct {
	TagName     string `json:"tag_name"`
	Description string `json:"description"`
}

func (t Tag) String() string {
	return Stringify(t)
}

// ListTagsOptions represents the available ListTags() options.
//
// GitLab API docs:
// https://docs.gitlab.com/ee/api/tags.html#list-project-repository-tags
type ListTagsOptions struct {
	ListOptions
	OrderBy *string `url:"order_by,omitempty" json:"order_by,omitempty"`
	Search  *string `url:"search,omitempty" json:"search,omitempty"`
	Sort    *string `url:"sort,omitempty" json:"sort,omitempty"`
}

// ListTags gets a list of tags from a project, sorted by name in reverse
// alphabetical order.
//
// GitLab API docs:
// https://docs.gitlab.com/ee/api/tags.html#list-project-repository-tags
func (s *TagsService) ListTags(pid interface{}, opt *ListTagsOptions, options ...RequestOptionFunc) ([]*Tag, *Response, error) {
	project, err := parseID(pid)
	if err != nil {
		return nil, nil, err
	}
	u := fmt.Sprintf("projects/%s/repository/tags", PathEscape(project))

	req, err := s.client.NewRequest(http.MethodGet, u, opt, options)
	if err != nil {
		return nil, nil, err
	}

	var t []*Tag
	resp, err := s.client.Do(req, &t)
	if err != nil {
		return nil, resp, err
	}

	return t, resp, nil
}

// GetTag a specific repository tag determined by its name. It returns 200 together
// with the tag information if the tag exists. It returns 404 if the tag does not exist.
//
// GitLab API docs:
// https://docs.gitlab.com/ee/api/tags.html#get-a-single-repository-tag
func (s *TagsService) GetTag(pid interface{}, tag string, options ...RequestOptionFunc) (*Tag, *Response, error) {
	project, err := parseID(pid)
	if err != nil {
		return nil, nil, err
	}
	u := fmt.Sprintf("projects/%s/repository/tags/%s", PathEscape(project), url.PathEscape(tag))

	req, err := s.client.NewRequest(http.MethodGet, u, nil, options)
	if err != nil {
		return nil, nil, err
	}

	var t *Tag
	resp, err := s.client.Do(req, &t)
	if err != nil {
		return nil, resp, err
	}

	return t, resp, nil
}

// CreateTagOptions represents the available CreateTag() options.
//
// GitLab API docs:
// https://docs.gitlab.com/ee/api/tags.html#create-a-new-tag
type CreateTagOptions struct {
	TagName *string `url:"tag_name,omitempty" json:"tag_name,omitempty"`
	Ref     *string `url:"ref,omitempty" json:"ref,omitempty"`
	Message *string `url:"message,omitempty" json:"message,omitempty"`

	// Deprecated: Use the Releases API instead. (Deprecated in GitLab 11.7)
	ReleaseDescription *string `url:"release_description:omitempty" json:"release_description,omitempty"`
}

// CreateTag creates a new tag in the repository that points to the supplied ref.
//
// GitLab API docs:
// https://docs.gitlab.com/ee/api/tags.html#create-a-new-tag
func (s *TagsService) CreateTag(pid interface{}, opt *CreateTagOptions, options ...RequestOptionFunc) (*Tag, *Response, error) {
	project, err := parseID(pid)
	if err != nil {
		return nil, nil, err
	}
	u := fmt.Sprintf("projects/%s/repository/tags", PathEscape(project))

	req, err := s.client.NewRequest(http.MethodPost, u, opt, options)
	if err != nil {
		return nil, nil, err
	}

	t := new(Tag)
	resp, err := s.client.Do(req, t)
	if err != nil {
		return nil, resp, err
	}

	return t, resp, nil
}

// DeleteTag deletes a tag of a repository with given name.
//
// GitLab API docs:
// https://docs.gitlab.com/ee/api/tags.html#delete-a-tag
func (s *TagsService) DeleteTag(pid interface{}, tag string, options ...RequestOptionFunc) (*Response, error) {
	project, err := parseID(pid)
	if err != nil {
		return nil, err
	}
	u := fmt.Sprintf("projects/%s/repository/tags/%s", PathEscape(project), url.PathEscape(tag))

	req, err := s.client.NewRequest(http.MethodDelete, u, nil, options)
	if err != nil {
		return nil, err
	}

	return s.client.Do(req, nil)
}

// CreateReleaseNoteOptions represents the available CreateReleaseNote() options.
//
// Deprecated: This feature was deprecated in GitLab 11.7.
//
// GitLab API docs:
// https://docs.gitlab.com/ee/api/tags.html#create-a-new-release
type CreateReleaseNoteOptions struct {
	Description *string `url:"description:omitempty" json:"description,omitempty"`
}

// CreateReleaseNote Add release notes to the existing git tag.
// If there already exists a release for the given tag, status code 409 is returned.
//
// Deprecated: This feature was deprecated in GitLab 11.7.
//
// GitLab API docs:
// https://docs.gitlab.com/ee/api/tags.html#create-a-new-release
func (s *TagsService) CreateReleaseNote(pid interface{}, tag string, opt *CreateReleaseNoteOptions, options ...RequestOptionFunc) (*ReleaseNote, *Response, error) {
	project, err := parseID(pid)
	if err != nil {
		return nil, nil, err
	}
	u := fmt.Sprintf("projects/%s/repository/tags/%s/release", PathEscape(project), url.PathEscape(tag))

	req, err := s.client.NewRequest(http.MethodPost, u, opt, options)
	if err != nil {
		return nil, nil, err
	}

	r := new(ReleaseNote)
	resp, err := s.client.Do(req, r)
	if err != nil {
		return nil, resp, err
	}

	return r, resp, nil
}

// UpdateReleaseNoteOptions represents the available UpdateReleaseNote() options.
//
// GitLab API docs:
// https://docs.gitlab.com/ee/api/tags.html#update-a-release
type UpdateReleaseNoteOptions struct {
	Description *string `url:"description:omitempty" json:"description,omitempty"`
}

// UpdateReleaseNote Updates the release notes of a given release.
//
// Deprecated: This feature was deprecated in GitLab 11.7.
//
// GitLab API docs:
// https://docs.gitlab.com/ee/api/tags.html#update-a-release
func (s *TagsService) UpdateReleaseNote(pid interface{}, tag string, opt *UpdateReleaseNoteOptions, options ...RequestOptionFunc) (*ReleaseNote, *Response, error) {
	project, err := parseID(pid)
	if err != nil {
		return nil, nil, err
	}
	u := fmt.Sprintf("projects/%s/repository/tags/%s/release", PathEscape(project), url.PathEscape(tag))

	req, err := s.client.NewRequest(http.MethodPut, u, opt, options)
	if err != nil {
		return nil, nil, err
	}

	r := new(ReleaseNote)
	resp, err := s.client.Do(req, r)
	if err != nil {
		return nil, resp, err
	}

	return r, resp, nil
}
