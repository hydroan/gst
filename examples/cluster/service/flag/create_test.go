package flag_test

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"cluster/helper"
	"cluster/internal/testsupport"
	"cluster/model"
	"cluster/pb"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/testutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// TestCreate covers POST /api/flags and the CreateFlag rpc, served by Creator
// in create.go: the hook fills in the percent of a flag turned on without
// one, over either transport, a flag without a name is refused, 400 over
// HTTP and InvalidArgument over gRPC, a request naming no session is refused
// before the service sees it, 401 and Unauthenticated, and a gRPC call
// answers with the x-served-by header the interceptor in
// interceptor/served_by.go adds to every call.
func TestCreate(t *testing.T) {
	ctx := testsupport.Authorized(t.Context(), testsupport.Session(t))
	t.Run("over HTTP", func(t *testing.T) {
		cli := testsupport.Login(t).Client

		flag, err := cli.Post[model.Flag](ctx, "/api/flags", &model.Flag{Name: flagName("http"), On: true})
		require.NoError(t, err)
		require.NotEmpty(t, flag.ID)
		require.Equal(t, 100, flag.Percent, "a flag turned on without a percent applies to everyone")
	})

	t.Run("over gRPC", func(t *testing.T) {
		flags := pb.NewFlagServiceClient(testsupport.Dial(t))

		var header metadata.MD
		rsp, err := flags.CreateFlag(ctx, &pb.CreateFlagRequest{Flag: &pb.Flag{Name: flagName("grpc"), On: true}}, grpc.Header(&header))
		require.NoError(t, err)
		require.NotEmpty(t, rsp.GetFlag().GetId())
		require.EqualValues(t, 100, rsp.GetFlag().GetPercent(), "the same hook runs over gRPC")
		require.Equal(t, []string{helper.Replica()}, header.Get("x-served-by"), "every call names the replica that served it")
	})

	t.Run("without a name", func(t *testing.T) {
		cli := testsupport.Login(t).Client
		_, err := cli.Post[model.Flag](ctx, "/api/flags", &model.Flag{On: true})
		testutil.RequireError(t, err, http.StatusBadRequest)

		flags := pb.NewFlagServiceClient(testsupport.Dial(t))
		_, err = flags.CreateFlag(ctx, &pb.CreateFlagRequest{Flag: &pb.Flag{On: true}})
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("without a session", func(t *testing.T) {
		cli, err := client.New(testutil.BaseURL())
		require.NoError(t, err)
		_, err = cli.Post[model.Flag](t.Context(), "/api/flags", &model.Flag{Name: flagName("anonymous")})
		testutil.RequireError(t, err, http.StatusUnauthorized)

		flags := pb.NewFlagServiceClient(testsupport.Dial(t))
		_, err = flags.CreateFlag(t.Context(), &pb.CreateFlagRequest{Flag: &pb.Flag{Name: flagName("anonymous")}})
		require.Equal(t, codes.Unauthenticated, status.Code(err))
	})
}

// TestGet covers the GetFlag rpc: a flag created over HTTP is read over gRPC,
// the two transports sharing one table, and an id no flag has answers
// NotFound.
func TestGet(t *testing.T) {
	ctx := testsupport.Authorized(t.Context(), testsupport.Session(t))
	cli := testsupport.Login(t).Client
	created, err := cli.Post[model.Flag](ctx, "/api/flags", &model.Flag{Name: flagName("get"), Note: "created over http"})
	require.NoError(t, err)

	flags := pb.NewFlagServiceClient(testsupport.Dial(t))
	rsp, err := flags.GetFlag(ctx, &pb.GetFlagRequest{Id: created.ID})
	require.NoError(t, err)
	require.Equal(t, created.Name, rsp.GetFlag().GetName())
	require.Equal(t, "created over http", rsp.GetFlag().GetNote())

	_, err = flags.GetFlag(ctx, &pb.GetFlagRequest{Id: "no-such-flag"})
	require.Equal(t, codes.NotFound, status.Code(err))
}

// TestList covers the ListFlag rpc: the filters, sorting and paging of the
// HTTP list, each filter a field, an operator and its values.
func TestList(t *testing.T) {
	ctx := testsupport.Authorized(t.Context(), testsupport.Session(t))
	flags := pb.NewFlagServiceClient(testsupport.Dial(t))
	note := flagName("list")
	names := []string{note + "-a", note + "-b", note + "-c"}
	for i, name := range names {
		_, err := flags.CreateFlag(ctx, &pb.CreateFlagRequest{Flag: &pb.Flag{Name: name, On: i != 1, Note: note}})
		require.NoError(t, err)
	}
	byNote := &pb.ListFlagRequest_Filter{Field: "note", Values: []string{note}}

	t.Run("filter by a field", func(t *testing.T) {
		rsp, err := flags.ListFlag(ctx, &pb.ListFlagRequest{Filters: []*pb.ListFlagRequest_Filter{byNote, {Field: "on", Values: []string{"true"}}}})
		require.NoError(t, err)
		require.EqualValues(t, 2, rsp.GetTotal())
		for _, item := range rsp.GetItems() {
			require.True(t, item.GetOn())
		}
	})

	t.Run("filter with an operator", func(t *testing.T) {
		rsp, err := flags.ListFlag(ctx, &pb.ListFlagRequest{Filters: []*pb.ListFlagRequest_Filter{{Field: "name", Op: "in", Values: names[:2]}}})
		require.NoError(t, err)
		require.EqualValues(t, 2, rsp.GetTotal())
	})

	t.Run("sort and page", func(t *testing.T) {
		rsp, err := flags.ListFlag(ctx, &pb.ListFlagRequest{Filters: []*pb.ListFlagRequest_Filter{byNote}, SortBy: []string{"name desc"}, Page: 1, Size: 2})
		require.NoError(t, err)
		require.EqualValues(t, 3, rsp.GetTotal(), "the total counts every match, not the page")
		require.Len(t, rsp.GetItems(), 2)
		require.Equal(t, names[2], rsp.GetItems()[0].GetName())
		require.Equal(t, names[1], rsp.GetItems()[1].GetName())
	})
}

// TestUpdate covers the UpdateFlag rpc: the whole flag is replaced.
func TestUpdate(t *testing.T) {
	ctx := testsupport.Authorized(t.Context(), testsupport.Session(t))
	flags := pb.NewFlagServiceClient(testsupport.Dial(t))
	name := flagName("update")
	created, err := flags.CreateFlag(ctx, &pb.CreateFlagRequest{Flag: &pb.Flag{Name: name, On: true}})
	require.NoError(t, err)

	rsp, err := flags.UpdateFlag(ctx, &pb.UpdateFlagRequest{Id: created.GetFlag().GetId(), Flag: &pb.Flag{Name: name, On: false, Percent: 20, Note: "updated"}})
	require.NoError(t, err)
	require.False(t, rsp.GetFlag().GetOn())
	require.EqualValues(t, 20, rsp.GetFlag().GetPercent())

	read, err := flags.GetFlag(ctx, &pb.GetFlagRequest{Id: created.GetFlag().GetId()})
	require.NoError(t, err)
	require.Equal(t, "updated", read.GetFlag().GetNote())
}

// TestPatch covers the PatchFlag rpc: update_mask names the fields to change
// and the rest keep their values; a mask naming nothing, or a field the flag
// does not have, is refused with InvalidArgument.
func TestPatch(t *testing.T) {
	ctx := testsupport.Authorized(t.Context(), testsupport.Session(t))
	flags := pb.NewFlagServiceClient(testsupport.Dial(t))
	created, err := flags.CreateFlag(ctx, &pb.CreateFlagRequest{Flag: &pb.Flag{Name: flagName("patch"), On: true}})
	require.NoError(t, err)
	id := created.GetFlag().GetId()

	rsp, err := flags.PatchFlag(ctx, &pb.PatchFlagRequest{Id: id, Flag: &pb.Flag{Percent: 5}, UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"percent"}}})
	require.NoError(t, err)
	require.EqualValues(t, 5, rsp.GetFlag().GetPercent())
	require.True(t, rsp.GetFlag().GetOn(), "a field outside the mask keeps its value")
	require.Equal(t, created.GetFlag().GetName(), rsp.GetFlag().GetName())

	_, err = flags.PatchFlag(ctx, &pb.PatchFlagRequest{Id: id, Flag: &pb.Flag{Percent: 7}})
	require.Equal(t, codes.InvalidArgument, status.Code(err), "a patch names the fields it changes")

	_, err = flags.PatchFlag(ctx, &pb.PatchFlagRequest{Id: id, Flag: &pb.Flag{Percent: 7}, UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"shade"}}})
	require.Equal(t, codes.InvalidArgument, status.Code(err), "a field the flag does not have is refused")
}

// TestDelete covers the DeleteFlag rpc: the flag is gone afterwards.
func TestDelete(t *testing.T) {
	ctx := testsupport.Authorized(t.Context(), testsupport.Session(t))
	flags := pb.NewFlagServiceClient(testsupport.Dial(t))
	created, err := flags.CreateFlag(ctx, &pb.CreateFlagRequest{Flag: &pb.Flag{Name: flagName("delete")}})
	require.NoError(t, err)
	id := created.GetFlag().GetId()

	_, err = flags.DeleteFlag(ctx, &pb.DeleteFlagRequest{Id: id})
	require.NoError(t, err)

	_, err = flags.GetFlag(ctx, &pb.GetFlagRequest{Id: id})
	require.Equal(t, codes.NotFound, status.Code(err))
}

// TestBatch covers the batch rpcs, CreateManyFlag, UpdateManyFlag,
// PatchManyFlag and DeleteManyFlag, over one set of flags: created together,
// updated together, patched together with a mask each, deleted together.
func TestBatch(t *testing.T) {
	ctx := testsupport.Authorized(t.Context(), testsupport.Session(t))
	flags := pb.NewFlagServiceClient(testsupport.Dial(t))
	note := flagName("batch")
	byNote := &pb.ListFlagRequest{Filters: []*pb.ListFlagRequest_Filter{{Field: "note", Values: []string{note}}}}

	created, err := flags.CreateManyFlag(ctx, &pb.CreateManyFlagRequest{Items: []*pb.Flag{{Name: note + "-a", Note: note}, {Name: note + "-b", Note: note}}})
	require.NoError(t, err)
	require.Len(t, created.GetItems(), 2)
	ids := []string{created.GetItems()[0].GetId(), created.GetItems()[1].GetId()}
	require.NotEmpty(t, ids[0])
	require.NotEmpty(t, ids[1])

	updated, err := flags.UpdateManyFlag(ctx, &pb.UpdateManyFlagRequest{Items: []*pb.Flag{
		{Id: ids[0], Name: note + "-a", On: true, Percent: 10, Note: note},
		{Id: ids[1], Name: note + "-b", On: true, Percent: 20, Note: note},
	}})
	require.NoError(t, err)
	require.Len(t, updated.GetItems(), 2)
	for _, item := range updated.GetItems() {
		require.True(t, item.GetOn())
	}

	patched, err := flags.PatchManyFlag(ctx, &pb.PatchManyFlagRequest{Items: []*pb.PatchManyFlagRequest_Item{
		{Flag: &pb.Flag{Id: ids[0], Percent: 50}, UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"percent"}}},
		{Flag: &pb.Flag{Id: ids[1], Percent: 60}, UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"percent"}}},
	}})
	require.NoError(t, err)
	require.Len(t, patched.GetItems(), 2)
	for _, item := range patched.GetItems() {
		require.True(t, item.GetOn(), "a field outside the mask keeps its value")
		require.Equal(t, note, item.GetNote())
	}

	listed, err := flags.ListFlag(ctx, byNote)
	require.NoError(t, err)
	require.EqualValues(t, 2, listed.GetTotal())

	_, err = flags.DeleteManyFlag(ctx, &pb.DeleteManyFlagRequest{Ids: ids})
	require.NoError(t, err)

	listed, err = flags.ListFlag(ctx, byNote)
	require.NoError(t, err)
	require.EqualValues(t, 0, listed.GetTotal())
}

// flagName returns a flag name no other test uses: the name is unique in the
// table, and every test of this package writes the one table.
func flagName(prefix string) string {
	return prefix + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
}
