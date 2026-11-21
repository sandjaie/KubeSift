# session

`session` owns background workers for an immutable target: a watch, a log follow, or a terminal. Kubernetes stream adapters and the desktop bridge start those workers. Closing the owning connection stops them. Removing a frontend listener does not cancel the backend worker.

`StartPending` followed by `Activate` holds events until the frontend has registered for that session, so nothing is delivered into a listener that does not exist yet. `Stop` waits for the worker and the dispatcher to finish. If they do not finish, it reports a timeout rather than forgetting the worker.

`buffer.go` bounds bytes waiting to cross the desktop bridge. A log flood drops bytes and reports truncation. A watch asks for a resync instead of growing without a limit. Terminal chunks require acknowledgements and apply backpressure. Terminal bytes wait, or the send fails explicitly. They are not discarded to make room in the queue.
