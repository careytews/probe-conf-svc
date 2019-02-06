
FROM fedora:28

RUN dnf update -y && dnf install -y libgo && dnf clean all

COPY service /usr/local/bin/

CMD /usr/local/bin/service

