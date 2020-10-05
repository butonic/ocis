package cs3

import (
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	v1beta11 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/token"
	idxerrs "github.com/owncloud/ocis/accounts/pkg/indexer/errors"
	"google.golang.org/grpc/metadata"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"fmt"
	"context"
	"strings"
	"github.com/cs3org/reva/pkg/token/manager/jwt"
)

type NonUnique struct {
	indexBy      string
	typeName     string
	filesDir     string
	indexBaseDir string
	indexRootDir string

	tokenManager    token.Manager
	storageProvider provider.ProviderAPIClient
	dataProvider    dataProviderClient // Used to create and download data via http, bypassing reva upload protocol

	cs3conf *Config
}

// NewNonUniqueIndex instantiates a new UniqueIndex instance. Init() should be
// called afterward to ensure correct on-disk structure.
//
// /var/tmp/ocis-accounts/index.cs3/Pets/Bro*
// ├── Brown/
// │   └── rebef-123 -> /var/tmp/testfiles-395764020/pets/rebef-123
// ├── Green/
// │    ├── goefe-789 -> /var/tmp/testfiles-395764020/pets/goefe-789
// │    └── xadaf-189 -> /var/tmp/testfiles-395764020/pets/xadaf-189
// └── White/
//     └── wefwe-456 -> /var/tmp/testfiles-395764020/pets/wefwe-456
func NewNonUniqueIndex(typeName, indexBy, filesDir, indexBaseDir string, cfg *Config) NonUnique {
	return NonUnique{
		indexBy:      indexBy,
		typeName:     typeName,
		filesDir:     filesDir,
		indexBaseDir: indexBaseDir,
		indexRootDir: path.Join(indexBaseDir, strings.Join([]string{"unique", typeName, indexBy}, ".")),
		cs3conf:      cfg,
		dataProvider: dataProviderClient{
			client: http.Client{
				Transport: http.DefaultTransport,
			},
		},
	}
}

func (idx *NonUnique) Init() error {
	tokenManager, err := jwt.New(map[string]interface{}{
		"secret": idx.cs3conf.JWTSecret,
	})

	if err != nil {
		return err
	}

	idx.tokenManager = tokenManager

	client, err := pool.GetStorageProviderServiceClient(idx.cs3conf.ProviderAddr)
	if err != nil {
		return err
	}

	idx.storageProvider = client

	ctx := context.Background()
	tk, err := idx.authenticate(ctx)
	if err != nil {
		return err
	}
	ctx = metadata.AppendToOutgoingContext(ctx, token.TokenHeader, tk)

	if err := idx.makeDirIfNotExists(ctx, idx.indexBaseDir); err != nil {
		return err
	}

	if err := idx.makeDirIfNotExists(ctx, idx.indexRootDir); err != nil {
		return err
	}

	return nil
}

func (idx NonUnique) Lookup(v string) ([]string, error) {
	panic("implement me")
}

func (idx NonUnique) Add(id, v string) (string, error) {
	newName := singleJoiningSlash(idx.cs3conf.DataURL, path.Join(idx.cs3conf.DataPrefix, idx.indexRootDir, v))
	if err := idx.makeDirIfNotExists(context.TODO(), newName); err != nil {
		return "", err

	}
	if err := idx.createSymlink(id, path.Join(newName, id)); err != nil {
		if os.IsExist(err) {
			return "", &idxerrs.AlreadyExistsErr{idx.typeName, idx.indexBy, v}
		}

		return "", err
	}

	return newName, nil
}

func (idx NonUnique) Remove(id string, v string) error {
	panic("implement me")
}

func (idx NonUnique) Update(id, oldV, newV string) error {
	panic("implement me")
}

func (idx NonUnique) Search(pattern string) ([]string, error) {
	panic("implement me")
}

func (idx NonUnique) IndexBy() string {
	panic("implement me")
}

func (idx NonUnique) TypeName() string {
	panic("implement me")
}

func (idx NonUnique) FilesDir() string {
	panic("implement me")
}

func (idx *NonUnique) authenticate(ctx context.Context) (token string, err error) {
	u := &user.User{
		Id:     &user.UserId{},
		Groups: []string{},
	}
	if idx.cs3conf.ServiceUserName != "" {
		u.Id.OpaqueId = idx.cs3conf.ServiceUserUUID
	}
	return idx.tokenManager.MintToken(ctx, u)
}

func (idx *NonUnique) makeDirIfNotExists(ctx context.Context, folder string) error {
	var rootPathRef = &provider.Reference{
		Spec: &provider.Reference_Path{Path: fmt.Sprintf("/meta/%v", folder)},
	}

	resp, err := idx.storageProvider.Stat(ctx, &provider.StatRequest{
		Ref: rootPathRef,
	})

	if err != nil {
		return err
	}

	if resp.Status.Code == v1beta11.Code_CODE_NOT_FOUND {
		_, err := idx.storageProvider.CreateContainer(ctx, &provider.CreateContainerRequest{
			Ref: rootPathRef,
		})

		if err != nil {
			return err
		}
	}

	return nil
}

func (idx *NonUnique) createSymlink(oldname, newname string) error {
	t, err := idx.authenticate(context.TODO())
	if err != nil {
		return err
	}

	if _, err := idx.resolveSymlink(newname); err == nil {
		return os.ErrExist
	}

	_, err = idx.dataProvider.put(newname, strings.NewReader(oldname), t)
	if err != nil {
		return err
	}

	return nil

}

func (idx *NonUnique) resolveSymlink(name string) (string, error) {
	t, err := idx.authenticate(context.TODO())
	if err != nil {
		return "", err
	}

	resp, err := idx.dataProvider.get(name, t)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return "", os.ErrNotExist
		}

		return "", fmt.Errorf("could not resolve symlink %s, got status %v", name, resp.StatusCode)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err

	}
	return string(b), err
}
