package agency

import (
	os_client "github.com/d0rc/agent-os/os-client"
	"time"
)

const NumberOfVotesToCache = 3
const VoterMinResults = 3
const MinimumNumberOfVotes = 3
const MinimalVotingRatingForCommand = 5.0
const ToTPathLenToTriggerTerminalCallback = 18
const ResubmitSystemPromptAfter = 15 * time.Minute
const MaxIoRequestsThreads = 16
const WriteVotesLog = false

const MaxJobsPerAgent = 12
const JobsManagerInferenceTimeout = 600 * time.Second
const JobsManagerExecutionPool = os_client.REP_Default
