package modeliamsession

// CurrentGetReq is the request payload for getting the current session.
type CurrentGetReq struct{}

// CurrentGetRsp returns the current session together with the latest principal snapshot.
type CurrentGetRsp = AuthenticatedSessionRsp

// CurrentDeleteReq is the request payload for deleting the current session.
type CurrentDeleteReq struct{}

// CurrentDeleteRsp is the response payload for deleting the current session.
type CurrentDeleteRsp struct{}
