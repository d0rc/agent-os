package agency

import "time"

const NumberOfVotesToCache = 3
const VoterMinResults = 7
const MinimumNumberOfVotes = 3
const MinimalVotingRatingForCommand = 3.0
const ToTPathLenToTriggerTerminalCallback = 14
const ResubmitSystemPromptAfter = 15 * time.Minute
const MaxIoRequestsThreads = 128
const WriteVotesLog = true
