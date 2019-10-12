package service

import (
	"../api"
	"../common"
	. "../component/client"
	"../component/profile"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"net/http"
	"time"
)

const (
	wsBufferSize = 1024 * 5
)

func HttpServer() {
	myRouter := mux.NewRouter().StrictSlash(true)

	//-----------------------

	myRouter.Handle("/admin", http.RedirectHandler("/admin/welcome", 301))
	myRouter.HandleFunc("/admin/welcome", handleWelcome)
	myRouter.HandleFunc("/admin/resources", handleResources)
	myRouter.HandleFunc("/admin/statistics", handleStatistics)
	myRouter.HandleFunc("/admin/options", handleOptions)
	myRouter.HandleFunc("/admin/logs", handleLogs)

	myRouter.Handle("/", http.RedirectHandler("/profile/welcome", 301))
	myRouter.Handle("/profile", http.RedirectHandler("/profile/welcome", 301))
	myRouter.HandleFunc("/profile/welcome", handleProfileWelcome)
	myRouter.HandleFunc("/profile/my", handleProfileMy)

	myRouter.PathPrefix("/resource").HandlerFunc(handleResource)
	myRouter.HandleFunc("/api", handleAPI)

	//-----------------------

	apiRouter := myRouter.PathPrefix("/v2/api").Subrouter()
	apiRouter.Use(handleCORS)
	apiRouter.HandleFunc("/auth", handleAuth).Methods("GET", "POST")
	apiRouter.Handle("/client", checkAuth(api.HandleGetClient)).Methods("GET")
	apiRouter.Handle("/chat", checkAuth(handleWS)) //handle for websocket from chat

	apiAdmin := apiRouter.PathPrefix("/admin").Subrouter()
	apiAdmin.Handle("/clients", checkAdmin(api.HandleGetClientsList)).Methods("GET")
	apiAdmin.Handle("/profiles", checkAdmin(api.HandleGetProfileList)).Methods("GET")
	apiAdmin.Handle("/log", checkAdmin(api.HandleGetLog)).Methods("GET")
	apiAdmin.Handle("/log", checkAdmin(api.HandleDelLog)).Methods("DELETE")

	//-----------------------

	go func() {
		err := http.ListenAndServe(":"+common.Options.HttpServerPort, myRouter)
		if err != nil {
			common.LogAdd(common.MessError, "webServer не смог занять порт: "+fmt.Sprint(err))
		}
	}()

	err := http.ListenAndServeTLS(":"+common.Options.HttpsServerPort, common.Options.HttpsCertPath, common.Options.HttpsKeyPath, myRouter)
	if err != nil {
		common.LogAdd(common.MessError, "webServer не смог занять порт: "+fmt.Sprint(err))
	}

}

func checkAuth(f func(w http.ResponseWriter, r *http.Request, client *Client)) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pidCookie, err := r.Cookie("abc")
		if err != nil {
			http.Error(w, "unknown user", http.StatusUnauthorized)
			return
		}

		tokenCookie, err := r.Cookie("cba")
		if err != nil {
			http.Error(w, "unknown user", http.StatusUnauthorized)
			return
		}

		list := GetClientsList(pidCookie.Value)
		for _, c := range list {
			if c.Token == tokenCookie.Value {
				pidCookie.Expires = time.Now().Add(common.WebSessionTimeoutHour * time.Hour)
				tokenCookie.Expires = time.Now().Add(common.WebSessionTimeoutHour * time.Hour)
				http.SetCookie(w, pidCookie)
				http.SetCookie(w, tokenCookie)
				f(w, r, c)
				return
			}
		}

		http.Error(w, "unknown user", http.StatusUnauthorized)
	})
}

func checkAdmin(f func(w http.ResponseWriter, r *http.Request)) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if ok {
			if user == common.Options.AdminLogin && pass == common.Options.AdminPass {
				f(w, r)
				return
			}
		}

		common.LogAdd(common.MessError, "WWW Аутентификация админки провалилась "+r.RemoteAddr)
		w.Header().Set("WWW-Authenticate", "Basic")
		http.Error(w, "auth req", http.StatusUnauthorized)
		return
	})
}

func handleCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		common.LogAdd(common.MessFull, "get req: "+r.RequestURI)

		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Max-Age", "6400")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, X-Requested-With, X-HTTP-Method-Override, Content-Type, Accept")

		if r.Method == "OPTIONS" {
			w.Write([]byte("ok"))
			return
		}

		h.ServeHTTP(w, r)
	})
}

func handleAuth(w http.ResponseWriter, r *http.Request) {
	pid := string(r.FormValue("abc"))
	token := string(r.FormValue("cba"))
	destination := string(r.FormValue("destination"))

	common.LogAdd(common.MessInfo, "trying to auth app "+pid)

	list := GetClientsList(pid)

	//--------- todo удалить
	if len(list) == 0 {
		newClient := &Client{Pid: pid, Pass:token, Token: token, Profile: profile.GetProfile("vaizmanai@mail.ru") }
		newClient.StoreClient()
	}
	//--------- todo удалить

	list = GetClientsList(pid)
	for _, c := range list {
		if c.Token == token {

			//--------- todo разрешить
			//clientIp, _, _ := net.SplitHostPort((*c.Conn).RemoteAddr().String())
			//webIp, _, _ := net.SplitHostPort(r.RemoteAddr)
			//if webIp != clientIp {
			//	continue
			//}
			//--------- todo разрешить

			cookie_pid := http.Cookie{Name: "abc", Value: pid, Expires: time.Now().Add(common.WebSessionTimeoutHour * time.Hour), Path: "/"}
			cookie_token := http.Cookie{Name: "cba", Value: token, Expires: time.Now().Add(common.WebSessionTimeoutHour * time.Hour), Path: "/"}
			http.SetCookie(w, &cookie_pid)
			http.SetCookie(w, &cookie_token)
			http.Redirect(w, r, destination, http.StatusTemporaryRedirect)
			return
		}
	}
	http.Error(w, "Launch by reVisit, please!", http.StatusUnauthorized)
}

func handleWS(w http.ResponseWriter, r *http.Request, client *Client) {
	conn, err := websocket.Upgrade(w, r, w.Header(), wsBufferSize, wsBufferSize)

	if err != nil {
		http.Error(w, "could not open websocket connection", http.StatusBadRequest)
	}
	go api.HandleChatWS(conn, client)
}
