// Copyright 2023 The Perses Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package user

import (
	"encoding/json"
	"fmt"

	"github.com/brunoga/deep"
	"github.com/labstack/echo/v4"
	"github.com/perses/perses/internal/api/authorization"
	"github.com/perses/perses/internal/api/crypto"
	apiInterface "github.com/perses/perses/internal/api/interface"
	"github.com/perses/perses/internal/api/interface/v1/user"
	"github.com/perses/perses/pkg/model/api"
	v1 "github.com/perses/perses/pkg/model/api/v1"
	"github.com/sirupsen/logrus"
)

type service struct {
	user.Service
	dao   user.DAO
	authz authorization.Authorization
}

func NewService(dao user.DAO, authz authorization.Authorization) user.Service {
	return &service{
		dao:   dao,
		authz: authz,
	}
}

func (s *service) Create(_ echo.Context, entity *v1.User) (*v1.PublicUser, error) {
	copyEntity, err := deep.Copy(entity)
	if err != nil {
		return nil, fmt.Errorf("failed to copy entity: %w", err)
	}
	return s.create(copyEntity)
}

func (s *service) create(entity *v1.User) (*v1.PublicUser, error) {
	// Update the time contains in the entity
	entity.Metadata.CreateNow()
	// check that the password is correctly filled
	if len(entity.Spec.NativeProvider.Password) == 0 {
		return nil, fmt.Errorf("%w: password cannot be empty", apiInterface.BadRequestError)
	}
	hash, err := crypto.HashAndSalt([]byte(entity.Spec.NativeProvider.Password))
	if err != nil {
		logrus.WithError(err).Errorf("unable to generate the hash for the password of the user %s", entity.Metadata.Name)
		return nil, apiInterface.InternalError
	}
	// save the hash in the password field
	entity.Spec.NativeProvider.Password = string(hash)
	if createErr := s.dao.Create(entity); createErr != nil {
		return nil, createErr
	}
	// Refreshing RBAC cache as the user's associated role may be updated, which can add or remove permissions.
	if err := s.authz.RefreshPermissions(); err != nil {
		logrus.WithError(err).Error("failed to refresh RBAC cache")
	}
	return v1.NewPublicUser(entity), nil
}

func (s *service) Update(_ echo.Context, entity *v1.User, parameters apiInterface.Parameters) (*v1.PublicUser, error) {
	copyEntity, err := deep.Copy(entity)
	if err != nil {
		return nil, fmt.Errorf("failed to copy entity: %w", err)
	}
	return s.update(copyEntity, parameters)
}

func (s *service) update(entity *v1.User, parameters apiInterface.Parameters) (*v1.PublicUser, error) {
	if entity.Metadata.Name != parameters.Name {
		logrus.Debugf("name in user '%s' and coming from the http request: '%s' doesn't match", entity.Metadata.Name, parameters.Name)
		return nil, fmt.Errorf("%w: metadata.name and the name in the http path request doesn't match", apiInterface.BadRequestError)
	}
	// find the previous version of the project
	oldEntity, err := s.dao.Get(parameters.Name)
	if err != nil {
		return nil, err
	}
	entity.Metadata.Update(oldEntity.Metadata)
	// in case the user updated his password, then we should hash it again, otherwise the old password should be kept
	if len(entity.Spec.NativeProvider.Password) > 0 {
		hash, hashErr := crypto.HashAndSalt([]byte(entity.Spec.NativeProvider.Password))
		if hashErr != nil {
			logrus.WithError(hashErr).Errorf("unable to generate the hash for the password of the user %q", entity.Metadata.Name)
			return nil, hashErr
		}
		entity.Spec.NativeProvider.Password = string(hash)
	} else {
		entity.Spec.NativeProvider.Password = oldEntity.Spec.NativeProvider.Password
	}
	// in case the user is updating the firstname / lastname, then it should be updated, otherwise the old one should be kept
	if len(entity.Spec.FirstName) == 0 {
		entity.Spec.FirstName = oldEntity.Spec.FirstName
	}
	if len(entity.Spec.LastName) == 0 {
		entity.Spec.LastName = oldEntity.Spec.LastName
	}
	if updateErr := s.dao.Update(entity); updateErr != nil {
		logrus.WithError(err).Errorf("unable to perform the update of the user %q", entity.Metadata.Name)
		return nil, updateErr
	}
	// Refreshing RBAC cache as the user's associated role may be updated, which can add or remove permissions.
	if err := s.authz.RefreshPermissions(); err != nil {
		logrus.WithError(err).Error("failed to refresh RBAC cache")
	}
	return v1.NewPublicUser(entity), nil
}

func (s *service) Delete(_ echo.Context, parameters apiInterface.Parameters) error {
	err := s.dao.Delete(parameters.Name)
	if err != nil {
		return err
	}
	// Refreshing RBAC cache as the user's associated role may be updated, which can add or remove permissions.
	if err := s.authz.RefreshPermissions(); err != nil {
		logrus.WithError(err).Error("failed to refresh RBAC cache")
	}
	return nil
}

func (s *service) Get(parameters apiInterface.Parameters) (*v1.PublicUser, error) {
	usr, err := s.dao.Get(parameters.Name)
	if err != nil {
		return nil, err
	}
	return v1.NewPublicUser(usr), nil
}

func (s *service) List(q *user.Query, _ apiInterface.Parameters) ([]*v1.PublicUser, error) {
	l, err := s.dao.List(q)
	if err != nil {
		return nil, err
	}
	result := make([]*v1.PublicUser, 0, len(l))
	for _, usr := range l {
		result = append(result, v1.NewPublicUser(usr))
	}
	return result, nil
}

func (s *service) RawList(_ *user.Query, _ apiInterface.Parameters) ([]json.RawMessage, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *service) MetadataList(q *user.Query, _ apiInterface.Parameters) ([]api.Entity, error) {
	return s.dao.MetadataList(q)
}

func (s *service) RawMetadataList(q *user.Query, _ apiInterface.Parameters) ([]json.RawMessage, error) {
	return s.dao.RawMetadataList(q)
}
