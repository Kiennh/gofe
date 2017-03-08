package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	fe "./fe"
	models "./models"
	settings "./settings"
	"github.com/go-macaron/binding"
	"github.com/go-macaron/cache"
	"github.com/go-macaron/session"
	macaron "gopkg.in/macaron.v1"
)

var DEFAULT_API_ERROR_RESPONSE = models.GenericResp{
	models.GenericRespBody{false, "Not Supported"},
}

type SessionInfo struct {
	User         string
	Password     string
	FileExplorer fe.FileExplorer
	Uid          string
}

func main() {
	configRuntime()
	startServer()
}

func configRuntime() {
	numCPU := runtime.NumCPU()
	runtime.GOMAXPROCS(numCPU)
	fmt.Printf("Running with %d CPUs\n", numCPU)
}

func startServer() {
	settings.Load()
	macaron.Classic()
	m := macaron.New()
	m.Use(macaron.Logger())
	m.Use(macaron.Recovery())
	if len(settings.Server.Statics) > 0 {
		m.Use(macaron.Statics(macaron.StaticOptions{
			Prefix:      "static",
			SkipLogging: false,
		}, settings.Server.Statics...))
	}
	m.Use(cache.Cacher())
	m.Use(session.Sessioner())
	m.Use(macaron.Renderer())
	m.Use(Contexter())

	m.Post("/api/_", binding.Bind(models.GenericReq{}), apiHandler)
	m.Post("/bridges/php/handler.php", binding.Bind(models.GenericParams{}), apiHandler)
	m.Get("/", mainHandler)
	m.Get("/login", loginHandler)
	m.Get("/api/download", downloadHandler)
	m.Post("/api/upload", uploadHandler)

	if settings.Server.Type == "http" {
		bind := strings.Split(settings.Server.Bind, ":")
		if len(bind) == 1 {
			m.Run(bind[0])
		}
		if len(bind) == 2 {
			m.Run(bind[0], bind[1])
		}
	}
}

func mainHandler(ctx *macaron.Context) {
	ctx.HTML(200, "index")
}

func loginHandler(ctx *macaron.Context) {
	ctx.HTML(200, "login")
}

func uploadHandler(ctx *macaron.Context, sessionInfo SessionInfo) {
	r := ctx.Req
	r.ParseMultipartForm(32 << 20)
	destination := r.MultipartForm.Value["destination"][0]
	for uploadFile, _ := range r.MultipartForm.File {
		file, handler, err := r.FormFile(uploadFile)
		if err != nil {
			ApiErrorResponse(ctx, 400, err)
			return
		}
		defer file.Close()

		tmpfile, err := ioutil.TempFile("", "gofe-upload-")
		if err != nil {
			ApiErrorResponse(ctx, 400, err)
			return
		}
		defer os.Remove(tmpfile.Name()) // clean up

		io.Copy(tmpfile, file)

		b, err := ioutil.ReadFile(tmpfile.Name())
		if err != nil {
			ApiErrorResponse(ctx, 400, err)
			return
		}

		err = sessionInfo.FileExplorer.Save(filepath.Join(destination, handler.Filename), b)
		if err != nil {
			ApiErrorResponse(ctx, 400, err)
			return
		}

	}
	ApiSuccessResponse(ctx, "")
}

func downloadHandler(ctx *macaron.Context) {
	log.Println("downloadHandler")
	var filePath = ctx.Query("path")
	var filename = filepath.Base(filePath)

	ctx.Resp.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	ctx.Resp.Header().Set("Content-Type", ctx.Req.Header.Get("Content-Type"))
	ctx.ServeFile(filePath)
}

func defaultHandler(ctx *macaron.Context) {
	ctx.JSON(200, DEFAULT_API_ERROR_RESPONSE)
}

func checkPath(path string) bool {
	return path == "" || strings.HasPrefix(path, settings.Backend.Home)
}

func apiHandler(c *macaron.Context, json models.GenericParams, sessionInfo SessionInfo) {
	validPath := checkPath(json.Path) && checkPath(json.Item) &&
		checkPath(json.NewPath) && checkPath(json.NewItemPath)
	for _, path := range json.Items {
		validPath = validPath && checkPath(path)
	}
	if !validPath {
		c.JSON(500, models.GenericRespBody{false, fmt.Sprintf("Path now allow")})
		return
	}

	if json.Mode == "list" {
		ls, err := sessionInfo.FileExplorer.ListDir(json.Path)
		if err == nil {
			c.JSON(200, models.ListDirResp{ls})
		} else {
			ApiErrorResponse(c, 400, err)
		}
	} else if json.Mode == "rename" { // path, newPath
		err := sessionInfo.FileExplorer.Move(json.Item, json.NewItemPath)
		if err == nil {
			ApiSuccessResponse(c, "")
		} else {
			ApiErrorResponse(c, 400, err)
		}
	} else if json.Mode == "copy" { // path, newPath
		err := sessionInfo.FileExplorer.Copy(json.Item, json.NewItemPath)
		if err == nil {
			ApiSuccessResponse(c, "")
		} else {
			ApiErrorResponse(c, 400, err)
		}
	} else if json.Mode == "remove" { // path
		for _, path := range json.Items {
			err := sessionInfo.FileExplorer.Delete(path)
			if err != nil {
				ApiErrorResponse(c, 400, err)
				return
			}
		}
		ApiSuccessResponse(c, "")
	} else if json.Mode == "savefile" { // content, path
		c.JSON(200, DEFAULT_API_ERROR_RESPONSE)
	} else if json.Mode == "edit" { // path
		err := sessionInfo.FileExplorer.Save(json.Item, []byte(json.Content))
		if err != nil {
			ApiErrorResponse(c, 400, err)
			return
		}
		ApiSuccessResponse(c, "")
	} else if json.Mode == "createFolder" { // name, path
		err := sessionInfo.FileExplorer.Mkdir(json.NewPath, "")
		if err == nil {
			ApiSuccessResponse(c, "")
		} else {
			ApiErrorResponse(c, 400, err)
		}
	} else if json.Mode == "changePermissions" { // path, perms, permsCode, recursive
		for _, path := range json.Items {
			err := sessionInfo.FileExplorer.Chmod(path, json.Perms, json.PermsCode, json.Recursive)
			if err != nil {
				ApiErrorResponse(c, 400, err)
				return
			}
		}
		ApiSuccessResponse(c, "")
	} else if json.Mode == "compress" { // path, destination
		c.JSON(200, DEFAULT_API_ERROR_RESPONSE)
	} else if json.Mode == "extract" { // path, destination, sourceFile
		c.JSON(200, DEFAULT_API_ERROR_RESPONSE)
	} else if json.Mode == "getContent" {
		b, err := sessionInfo.FileExplorer.ReadFile(json.Item)
		if err != nil {
			c.JSON(500, models.GenericRespBody{false, fmt.Sprintf("File error : %s", json.Path)})
		} else {
			c.JSON(200, models.GetContentResp{string(b[:])})
		}
	} else if json.Mode == "move" {
		for _, path := range json.Items {
			name := filepath.Base(path)
			err := sessionInfo.FileExplorer.Move(path, filepath.Join(json.NewPath, name))
			if err != nil {
				ApiErrorResponse(c, 400, err)
				return
			}
		}
		ApiSuccessResponse(c, "")
	}
}

func IsApiPath(url string) bool {
	return strings.HasPrefix(url, "/api/") || strings.HasPrefix(url, "/bridges/php/handler.php")
}

func Contexter() macaron.Handler {
	return func(c *macaron.Context, cache cache.Cache, session session.Store, f *session.Flash) {
		isSigned := false
		sessionInfo := SessionInfo{}
		uid := session.Get("uid")

		if uid == nil {
			isSigned = false
		} else {
			sessionInfoObj := cache.Get(uid.(string))
			if sessionInfoObj == nil {
				isSigned = false
			} else {
				sessionInfo = sessionInfoObj.(SessionInfo)
				if sessionInfo.User == "" || sessionInfo.Password == "" {
					isSigned = false
				} else {
					isSigned = true
					c.Data["User"] = sessionInfo.User
					c.Map(sessionInfo)
					if sessionInfo.FileExplorer == nil {
						fe, err := BackendConnect(sessionInfo.User, sessionInfo.Password)
						sessionInfo.FileExplorer = fe
						if err != nil {
							isSigned = false
							if IsApiPath(c.Req.URL.Path) {
								ApiErrorResponse(c, 500, err)
							} else {
								AuthError(c, f, err)
							}
						}
					}
				}
			}
		}

		if isSigned == false {
			if strings.HasPrefix(c.Req.URL.Path, "/login") {
				if c.Req.Method == "POST" {
					username := c.Query("username")
					password := c.Query("password")
					fe, err := BackendConnect(username, password)
					if err != nil {
						AuthError(c, f, err)
					} else {
						uid := username // TODO: ??
						sessionInfo = SessionInfo{username, password, fe, uid}
						cache.Put(uid, sessionInfo, 100000000000)
						session.Set("uid", uid)
						c.Data["User"] = sessionInfo.User
						c.Map(sessionInfo)
						c.Redirect("/")
					}
				}
			} else {
				c.Redirect("/login")
			}
		} else {
			if strings.HasPrefix(c.Req.URL.Path, "/logout") {
				sessionInfo.FileExplorer.Close()
				session.Delete("uid")
				cache.Delete(uid.(string))
				c.SetCookie("MacaronSession", "")
				c.Redirect("/login")
			}
		}
	}
}

func BackendConnect(username string, password string) (fe.FileExplorer, error) {
	fe := fe.NewSSHFileExplorer(settings.Backend.Host, username, password)
	err := fe.Init()
	if err == nil {
		return fe, nil
	}
	log.Println(err)
	return nil, err
}

func ApiErrorResponse(c *macaron.Context, code int, obj interface{}) {
	var message string
	if err, ok := obj.(error); ok {
		message = err.Error()
	} else {
		message = obj.(string)
	}
	c.JSON(code, models.GenericResp{models.GenericRespBody{false, message}})
}

func ApiSuccessResponse(c *macaron.Context, message string) {
	c.JSON(200, models.GenericResp{models.GenericRespBody{true, message}})
}

func AuthError(c *macaron.Context, f *session.Flash, err error) {
	f.Set("ErrorMsg", err.Error())
	c.Data["Flash"] = f
	c.Data["ErrorMsg"] = err.Error()
	c.Redirect("/login")
}
