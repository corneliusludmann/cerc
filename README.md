# cerc
Cerc (circle in Romanian) monitors services by calling an HTTP endpoint with a token expecting to receive that token back within a certain amount of time.
Compared to regularly polling some health probe HTTP endpoint this method allows going full-circle: trigger some action which is answered by the some token.

**Beware:** this is just an experiment at this point and not ready for prime time.
