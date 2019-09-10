FROM scratch

COPY examples/selftest.json selftest.json
COPY cerc /

ENTRYPOINT [ "/cerc" ]
CMD [ "selftest.json" ]