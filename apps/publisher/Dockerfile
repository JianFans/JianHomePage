FROM node:25-bookworm-slim

WORKDIR /workspace
COPY --chown=node:node . .
RUN corepack enable && corepack install && pnpm install --frozen-lockfile

# The immutable image stamp prevents queued work from silently using different source.
ARG CODE_REVISION
RUN CODE_REVISION="$CODE_REVISION" node --input-type=module -e "import {writeFileSync} from 'node:fs'; const revision=process.env.CODE_REVISION; if(!/^[a-f0-9]{40}$/.test(revision||'')) throw new Error('Full CODE_REVISION required'); writeFileSync('.publisher-revision',revision)"
RUN chown -R node:node /workspace

ENV NODE_ENV=production
ENV PUBLISHER_HOST=0.0.0.0
ENV PUBLISHER_PORT=8091
ENV PUBLISHER_WORK_ROOT=/tmp/yujian-publisher
ENV PUBLISHER_CODE_REVISION=$CODE_REVISION
EXPOSE 8091
USER node
ENTRYPOINT ["node", "apps/publisher/src/main.mjs"]
