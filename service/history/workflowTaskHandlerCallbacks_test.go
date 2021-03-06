// The MIT License
//
// Copyright (c) 2020 Temporal Technologies Inc.  All rights reserved.
//
// Copyright (c) 2020 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package history

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/uber-go/tally"
	enumspb "go.temporal.io/api/enums/v1"
	querypb "go.temporal.io/api/query/v1"

	"go.temporal.io/server/api/persistenceblobs/v1"
	"go.temporal.io/server/common/log/loggerimpl"
	"go.temporal.io/server/common/metrics"
	"go.temporal.io/server/common/payloads"
)

type (
	WorkflowTaskHandlerCallbackSuite struct {
		*require.Assertions
		suite.Suite

		controller *gomock.Controller

		workflowTaskHandlerCallback *workflowTaskHandlerCallbacksImpl
		queryRegistry               queryRegistry
		mockMutableState            *MockmutableState
	}
)

func TestWorkflowTaskHandlerCallbackSuite(t *testing.T) {
	suite.Run(t, new(WorkflowTaskHandlerCallbackSuite))
}

func (s *WorkflowTaskHandlerCallbackSuite) SetupTest() {
	s.Assertions = require.New(s.T())
	s.controller = gomock.NewController(s.T())

	s.workflowTaskHandlerCallback = &workflowTaskHandlerCallbacksImpl{
		metricsClient: metrics.NewClient(tally.NoopScope, metrics.History),
		config:        NewDynamicConfigForTest(),
		logger:        loggerimpl.NewNopLogger(),
	}
	s.queryRegistry = s.constructQueryRegistry(10)
	s.mockMutableState = NewMockmutableState(s.controller)
	s.mockMutableState.EXPECT().GetQueryRegistry().Return(s.queryRegistry)
	s.mockMutableState.EXPECT().GetExecutionInfo().Return(&persistenceblobs.WorkflowExecutionInfo{
		WorkflowId: testWorkflowID,
	}).AnyTimes()
	s.mockMutableState.EXPECT().GetExecutionState().Return(&persistenceblobs.WorkflowExecutionState{
		RunId: testRunID,
	}).AnyTimes()
}

func (s *WorkflowTaskHandlerCallbackSuite) TearDownTest() {
	s.controller.Finish()
}

func (s *WorkflowTaskHandlerCallbackSuite) TestHandleBufferedQueries_HeartbeatWorkflowTask() {
	s.assertQueryCounts(s.queryRegistry, 10, 0, 0, 0)
	queryResults := s.constructQueryResults(s.queryRegistry.getBufferedIDs()[0:5], 10)
	s.workflowTaskHandlerCallback.handleBufferedQueries(s.mockMutableState, queryResults, false, testGlobalNamespaceEntry, true)
	s.assertQueryCounts(s.queryRegistry, 10, 0, 0, 0)
}

func (s *WorkflowTaskHandlerCallbackSuite) TestHandleBufferedQueries_NewWorkflowTask() {
	s.assertQueryCounts(s.queryRegistry, 10, 0, 0, 0)
	queryResults := s.constructQueryResults(s.queryRegistry.getBufferedIDs()[0:5], 10)
	s.workflowTaskHandlerCallback.handleBufferedQueries(s.mockMutableState, queryResults, true, testGlobalNamespaceEntry, false)
	s.assertQueryCounts(s.queryRegistry, 5, 5, 0, 0)
}

func (s *WorkflowTaskHandlerCallbackSuite) TestHandleBufferedQueries_NoNewWorkflowTask() {
	s.assertQueryCounts(s.queryRegistry, 10, 0, 0, 0)
	queryResults := s.constructQueryResults(s.queryRegistry.getBufferedIDs()[0:5], 10)
	s.workflowTaskHandlerCallback.handleBufferedQueries(s.mockMutableState, queryResults, false, testGlobalNamespaceEntry, false)
	s.assertQueryCounts(s.queryRegistry, 0, 5, 5, 0)
}

func (s *WorkflowTaskHandlerCallbackSuite) TestHandleBufferedQueries_QueryTooLarge() {
	s.assertQueryCounts(s.queryRegistry, 10, 0, 0, 0)
	bufferedIDs := s.queryRegistry.getBufferedIDs()
	queryResults := s.constructQueryResults(bufferedIDs[0:5], 10)
	largeQueryResults := s.constructQueryResults(bufferedIDs[5:10], 10*1024*1024)
	for k, v := range largeQueryResults {
		queryResults[k] = v
	}
	s.workflowTaskHandlerCallback.handleBufferedQueries(s.mockMutableState, queryResults, false, testGlobalNamespaceEntry, false)
	s.assertQueryCounts(s.queryRegistry, 0, 5, 0, 5)
}

func (s *WorkflowTaskHandlerCallbackSuite) constructQueryResults(ids []string, resultSize int) map[string]*querypb.WorkflowQueryResult {
	results := make(map[string]*querypb.WorkflowQueryResult)
	for _, id := range ids {
		results[id] = &querypb.WorkflowQueryResult{
			ResultType: enumspb.QUERY_RESULT_TYPE_ANSWERED,
			Answer:     payloads.EncodeBytes(make([]byte, resultSize, resultSize)),
		}
	}
	return results
}

func (s *WorkflowTaskHandlerCallbackSuite) constructQueryRegistry(numQueries int) queryRegistry {
	queryRegistry := newQueryRegistry()
	for i := 0; i < numQueries; i++ {
		queryRegistry.bufferQuery(&querypb.WorkflowQuery{})
	}
	return queryRegistry
}

func (s *WorkflowTaskHandlerCallbackSuite) assertQueryCounts(queryRegistry queryRegistry, buffered, completed, unblocked, failed int) {
	s.Len(queryRegistry.getBufferedIDs(), buffered)
	s.Len(queryRegistry.getCompletedIDs(), completed)
	s.Len(queryRegistry.getUnblockedIDs(), unblocked)
	s.Len(queryRegistry.getFailedIDs(), failed)
}
