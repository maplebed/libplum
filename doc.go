/*
Package libplum is a high level client library for interacting with PlumLife and the Lightpad smart switches.

Summary

libplumraw wraps HTTP calls to the Plum website and lightpads. This library
wraps that, abstracting out the raw HTTP calls and letting your interact more
easily with thinsg like houses and loads.

It is partially finished; it can successfully turn lights on and off and
interact with the motion sensor, but it can't yet set configs or adjust the glow
ring.

It must be configured with authentication tokens before it can be used.

*/
package libplum
