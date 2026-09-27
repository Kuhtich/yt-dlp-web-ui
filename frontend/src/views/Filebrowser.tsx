import {
  Backdrop,
  Box,
  Button,
  Card,
  CardActionArea,
  CardActions,
  CardContent,
  Checkbox,
  CircularProgress,
  Container,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  IconButton,
  List,
  ListItem,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  MenuItem,
  MenuList,
  Paper,
  SpeedDial,
  SpeedDialAction,
  SpeedDialIcon,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TableSortLabel,
  ToggleButton,
  ToggleButtonGroup,
  Tooltip,
  Typography
} from '@mui/material'

import ArrowUpwardIcon from '@mui/icons-material/ArrowUpward'
import DeleteForeverIcon from '@mui/icons-material/DeleteForever'
import DownloadIcon from '@mui/icons-material/Download'
import FolderIcon from '@mui/icons-material/Folder'
import GridViewIcon from '@mui/icons-material/GridView'
import InsertDriveFileIcon from '@mui/icons-material/InsertDriveFile'
import MusicNoteIcon from '@mui/icons-material/MusicNote'
import SaveAltIcon from '@mui/icons-material/SaveAlt'
import TableRowsIcon from '@mui/icons-material/TableRows'
import VideoFileIcon from '@mui/icons-material/VideoFile'
import ViewListIcon from '@mui/icons-material/ViewList'

import { matchW } from 'fp-ts/lib/TaskEither'
import { pipe } from 'fp-ts/lib/function'
import { useEffect, useMemo, useState, useTransition } from 'react'
import { useNavigate } from 'react-router-dom'
import { BehaviorSubject, Subject, combineLatestWith, map, share } from 'rxjs'
import { serverURL } from '../atoms/settings'
import { useObservable } from '../hooks/observable'
import { useToast } from '../hooks/toast'
import { useI18n } from '../hooks/useI18n'
import { ffetch } from '../lib/httpClient'
import { DirectoryEntry } from '../types'
import { base64URLEncode, formatSize } from '../utils'
import { useAtomValue } from 'jotai'

type Entry = DirectoryEntry & { selected: boolean }

type ViewMode = 'list' | 'table' | 'icons'

type ViewProps = {
  files: Entry[]
  t: (key: string) => string
  thumbURL: (path: string) => string
  onOpen: (path: string) => void
  onFolder: (path: string) => void
  onDownload: (path: string) => void
  onDelete: (entry: DirectoryEntry) => void
  onSelect: (name: string) => void
  onContextMenu: (e: React.MouseEvent, entry: DirectoryEntry) => void
}

const VIEW_KEY = 'filebrowserView'

export default function Downloaded() {
  const [menuPos, setMenuPos] = useState({ x: 0, y: 0 })
  const [showMenu, setShowMenu] = useState(false)
  const [currentFile, setCurrentFile] = useState<DirectoryEntry>()

  const [view, setView] = useState<ViewMode>(() => {
    const stored = localStorage.getItem(VIEW_KEY)
    return stored === 'table' || stored === 'icons' ? stored : 'list'
  })

  const serverAddr = useAtomValue(serverURL)
  const navigate = useNavigate()

  const { i18n } = useI18n()
  const { pushMessage } = useToast()

  const [openDialog, setOpenDialog] = useState(false)

  const files$ = useMemo(() => new Subject<DirectoryEntry[]>(), [])
  const selected$ = useMemo(() => new BehaviorSubject<string[]>([]), [])

  const [isPending, startTransition] = useTransition()

  const fetcher = () => pipe(
    ffetch<DirectoryEntry[]>(
      `${serverAddr}/filebrowser/downloaded`,
      {
        method: 'POST',
        body: JSON.stringify({
          subdir: '',
        })
      }
    ),
    matchW(
      (e) => {
        pushMessage(e, 'error')
        navigate('/login')
      },
      (d) => files$.next(d ?? []),
    )
  )()

  const fetcherSubfolder = (sub: string) => {
    const folders = sub.startsWith('/')
      ? sub.substring(1).split('/')
      : sub.split('/')

    const relpath = folders.length >= 2
      ? folders.slice(-(folders.length - 1)).join('/')
      : folders.pop()

    const _upperLevel = folders.slice(1, -1)
    const upperLevel = _upperLevel.length === 2
      ? ['.', ..._upperLevel].join('/')
      : _upperLevel.join('/')

    const task = ffetch<DirectoryEntry[]>(`${serverAddr}/filebrowser/downloaded`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ subdir: relpath })
    })

    pipe(
      task,
      matchW(
        (l) => pushMessage(l, 'error'),
        (r) => files$.next(sub
          ? [{
            isDirectory: true,
            isVideo: false,
            isImage: false,
            isAudio: false,
            modTime: '',
            name: '..',
            path: upperLevel,
            size: 0,
          }, ...r.filter(f => f.name !== '')]
          : r.filter(f => f.name !== '')
        )
      )
    )()
  }

  const selectable$ = useMemo(() => files$.pipe(
    combineLatestWith(selected$),
    map(([data, selected]) => data.map(x => ({
      ...x,
      selected: selected.includes(x.name)
    }))),
    share()
  ), [])

  const selectable = useObservable(selectable$, [])

  const addSelected = (name: string) => {
    selected$.value.includes(name)
      ? selected$.next(selected$.value.filter(val => val !== name))
      : selected$.next([...selected$.value, name])
  }

  const deleteFile = (entry: DirectoryEntry) => pipe(
    ffetch(`${serverAddr}/filebrowser/delete`, {
      method: 'POST',
      body: JSON.stringify({
        path: entry.path,
      })
    }),
    matchW(
      (l) => pushMessage(l, 'error'),
      (_) => fetcher()
    )
  )()

  const deleteSelected = () => {
    Promise.all(selectable
      .filter(entry => entry.selected)
      .map(deleteFile)
    ).then(fetcher)
  }

  useEffect(() => {
    fetcher()
  }, [serverAddr])

  const onFileClick = (path: string) => startTransition(() => {
    const encoded = base64URLEncode(path)

    window.open(`${serverAddr}/filebrowser/v/${encoded}?token=${localStorage.getItem('token')}`)
  })

  const downloadFile = (path: string) => startTransition(() => {
    const encoded = base64URLEncode(path)

    window.open(`${serverAddr}/filebrowser/d/${encoded}?token=${localStorage.getItem('token')}`)
  })

  const onFolderClick = (path: string) => startTransition(() => {
    fetcherSubfolder(path)
  })

  const thumbURL = (path: string) =>
    `${serverAddr}/filebrowser/thumb/${base64URLEncode(path)}?token=${localStorage.getItem('token') ?? ''}`

  const viewProps: ViewProps = {
    files: selectable,
    t: i18n.t.bind(i18n),
    thumbURL,
    onOpen: (path) => onFileClick(path),
    onFolder: (path) => onFolderClick(path),
    onDownload: (path) => downloadFile(path),
    onDelete: (entry) => deleteFile(entry),
    onSelect: (name) => addSelected(name),
    onContextMenu: (e, entry) => {
      e.preventDefault()
      setCurrentFile(entry)
      setMenuPos({ x: e.clientX, y: e.clientY })
      setShowMenu(true)
    }
  }

  return (
    <Container
      maxWidth="xl"
      sx={{ mt: 4, mb: 4, minHeight: '100%' }}
      onClick={() => setShowMenu(false)}
    >
      <IconMenu
        posX={menuPos.x}
        posY={menuPos.y}
        hide={!showMenu}
        onDownload={() => {
          if (currentFile) {
            downloadFile(currentFile?.path)
            setCurrentFile(undefined)
          }
        }}
        onDelete={() => {
          if (currentFile) {
            deleteFile(currentFile)
            setCurrentFile(undefined)
          }
        }}
      />
      <Backdrop
        sx={{ color: '#fff', zIndex: (theme) => theme.zIndex.drawer + 1 }}
        open={!(files$.observed) || isPending}
      >
        <CircularProgress color="primary" />
      </Backdrop>
      <Paper
        sx={{
          p: 2,
          display: 'flex',
          flexDirection: 'column',
        }}
        onClick={() => setShowMenu(false)}
      >
        <Stack direction="row" justifyContent="flex-end" sx={{ mb: 1 }}>
          <ToggleButtonGroup
            size="small"
            exclusive
            value={view}
            onChange={(_, next: ViewMode | null) => {
              if (next) {
                setView(next)
                localStorage.setItem(VIEW_KEY, next)
              }
            }}
          >
            <ToggleButton value="list" aria-label={i18n.t('viewList')}>
              <Tooltip title={i18n.t('viewList')}>
                <ViewListIcon fontSize="small" />
              </Tooltip>
            </ToggleButton>
            <ToggleButton value="table" aria-label={i18n.t('viewTable')}>
              <Tooltip title={i18n.t('viewTable')}>
                <TableRowsIcon fontSize="small" />
              </Tooltip>
            </ToggleButton>
            <ToggleButton value="icons" aria-label={i18n.t('viewIcons')}>
              <Tooltip title={i18n.t('viewIcons')}>
                <GridViewIcon fontSize="small" />
              </Tooltip>
            </ToggleButton>
          </ToggleButtonGroup>
        </Stack>
        {selectable.length === 0 && (
          <Typography sx={{ p: 2 }}>{i18n.t('noFilesFound')}</Typography>
        )}
        {view === 'list' && <FilesList {...viewProps} />}
        {view === 'table' && <FilesTable {...viewProps} />}
        {view === 'icons' && <FilesIcons {...viewProps} />}
      </Paper>
      <SpeedDial
        ariaLabel='archive actions'
        sx={{ position: 'absolute', bottom: 64, right: 24 }}
        icon={<SpeedDialIcon />}
      >
        <SpeedDialAction
          icon={<DeleteForeverIcon />}
          tooltipTitle={i18n.t('deleteSelected')}
          tooltipOpen
          onClick={() => {
            if (selected$.value.length > 0) {
              setOpenDialog(true)
            }
          }}
        />
      </SpeedDial>
      <Dialog
        open={openDialog}
        onClose={() => setOpenDialog(false)}
      >
        <DialogTitle>
          Are you sure?
        </DialogTitle>
        <DialogContent>
          <DialogContentText id="alert-dialog-description">
            You're deleting:
          </DialogContentText>
          <ul>
            {selected$.value.map((entry, idx) => (
              <li key={idx}>{entry}</li>
            ))}
          </ul>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setOpenDialog(false)}>
            Cancel
          </Button>
          <Button
            onClick={() => {
              deleteSelected()
              setOpenDialog(false)
            }}
            autoFocus
          >
            Ok
          </Button>
        </DialogActions>
      </Dialog>
    </Container>
  )
}

const entryIcon = (file: DirectoryEntry, size = 24) => {
  if (file.isDirectory) {
    return file.name === '..'
      ? <ArrowUpwardIcon sx={{ fontSize: size }} color="disabled" />
      : <FolderIcon sx={{ fontSize: size }} color="primary" />
  }
  if (file.isVideo) {
    return <VideoFileIcon sx={{ fontSize: size }} />
  }
  if (file.isAudio) {
    return <MusicNoteIcon sx={{ fontSize: size }} />
  }
  return <InsertDriveFileIcon sx={{ fontSize: size }} />
}

const typeLabel = (file: DirectoryEntry, t: (key: string) => string) => {
  if (file.isDirectory) {
    return t('typeFolder')
  }
  if (file.isVideo) {
    return t('typeVideo')
  }
  if (file.isAudio) {
    return t('typeAudio')
  }
  if (file.isImage) {
    return t('typeImage')
  }
  return t('typeFile')
}

/** Renders the preview of a single tile: a frame from the video, the image
 * itself, or an icon when no preview can be produced. */
const Preview: React.FC<{ file: DirectoryEntry, thumbURL: (path: string) => string }> = ({ file, thumbURL }) => {
  const [failed, setFailed] = useState(false)

  if (file.name === '..' || file.isDirectory) {
    return (
      <Stack alignItems="center" justifyContent="center" sx={{ height: '100%' }}>
        {entryIcon(file, 56)}
      </Stack>
    )
  }

  if (file.isAudio) {
    return (
      <Stack alignItems="center" justifyContent="center" sx={{ height: '100%' }}>
        <MusicNoteIcon sx={{ fontSize: 56 }} />
      </Stack>
    )
  }

  if (failed) {
    return (
      <Stack alignItems="center" justifyContent="center" sx={{ height: '100%' }}>
        {entryIcon(file, 56)}
      </Stack>
    )
  }

  return (
    <Box
      component="img"
      src={thumbURL(file.path)}
      loading="lazy"
      onError={() => setFailed(true)}
      sx={{
        width: '100%',
        height: '100%',
        objectFit: 'cover',
        display: 'block',
      }}
    />
  )
}

const FilesList: React.FC<ViewProps> = ({ files, t, thumbURL, onOpen, onFolder, onDownload, onDelete, onSelect, onContextMenu }) => (
  <List sx={{ width: '100%', bgcolor: 'background.paper' }}>
    {files.map((file, idx) => (
      <ListItem
        onContextMenu={(e) => onContextMenu(e, file)}
        key={idx}
        secondaryAction={
          <Stack direction="row" alignItems="center" spacing={0.5}>
            {!file.isDirectory && (
              <Typography variant="caption" component="span">
                {formatSize(file.size)}
              </Typography>
            )}
            {!file.isDirectory && (
              <>
                <Tooltip title={t('download')}>
                  <IconButton size="small" onClick={() => onDownload(file.path)}>
                    <SaveAltIcon fontSize="small" />
                  </IconButton>
                </Tooltip>
                <Checkbox
                  edge="end"
                  checked={file.selected}
                  onChange={() => onSelect(file.name)}
                />
              </>
            )}
          </Stack>
        }
        disablePadding
      >
        <ListItemButton onClick={() => file.isDirectory ? onFolder(file.path) : onOpen(file.path)}>
          <ListItemIcon sx={{ minWidth: 64 }}>
            <Box sx={{
              width: 40,
              height: 40,
              borderRadius: 1,
              overflow: 'hidden',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              bgcolor: 'action.hover',
            }}>
              {file.isVideo || file.isImage
                ? <Preview file={file} thumbURL={thumbURL} />
                : entryIcon(file)}
            </Box>
          </ListItemIcon>
          <ListItemText
            primary={file.name}
            secondary={file.name !== '..' && new Date(file.modTime).toLocaleString()}
          />
        </ListItemButton>
      </ListItem>
    ))}
  </List>
)

type SortKey = 'name' | 'size' | 'modTime'

const FilesTable: React.FC<ViewProps> = ({ files, t, thumbURL, onOpen, onFolder, onDownload, onDelete, onSelect, onContextMenu }) => {
  const [orderBy, setOrderBy] = useState<SortKey>('modTime')
  const [desc, setDesc] = useState(true)

  const sorted = useMemo(() => {
    const cmp = (a: Entry, b: Entry) => {
      switch (orderBy) {
        case 'name':
          return a.name.localeCompare(b.name)
        case 'size':
          return a.size - b.size
        default:
          return new Date(a.modTime).getTime() - new Date(b.modTime).getTime()
      }
    }

    const head = files.filter(f => f.name === '..')
    const rest = files.filter(f => f.name !== '..')

    rest.sort((a, b) => desc ? -cmp(a, b) : cmp(a, b))

    return [...head, ...rest]
  }, [files, orderBy, desc])

  const sortBy = (key: SortKey) => {
    if (key === orderBy) {
      setDesc(!desc)
      return
    }
    setOrderBy(key)
    setDesc(key !== 'name')
  }

  return (
    <TableContainer>
      <Table size="small" sx={{ minWidth: 640 }}>
        <TableHead>
          <TableRow>
            <TableCell padding="checkbox" />
            <TableCell sortDirection={orderBy === 'name' ? (desc ? 'desc' : 'asc') : false}>
              <TableSortLabel
                active={orderBy === 'name'}
                direction={orderBy === 'name' && desc ? 'desc' : 'asc'}
                onClick={() => sortBy('name')}
              >
                {t('columnName')}
              </TableSortLabel>
            </TableCell>
            <TableCell align="right" sortDirection={orderBy === 'size' ? (desc ? 'desc' : 'asc') : false}>
              <TableSortLabel
                active={orderBy === 'size'}
                direction={orderBy === 'size' && desc ? 'desc' : 'asc'}
                onClick={() => sortBy('size')}
              >
                {t('columnSize')}
              </TableSortLabel>
            </TableCell>
            <TableCell sortDirection={orderBy === 'modTime' ? (desc ? 'desc' : 'asc') : false}>
              <TableSortLabel
                active={orderBy === 'modTime'}
                direction={orderBy === 'modTime' && desc ? 'desc' : 'asc'}
                onClick={() => sortBy('modTime')}
              >
                {t('columnModified')}
              </TableSortLabel>
            </TableCell>
            <TableCell>{t('columnType')}</TableCell>
            <TableCell align="right">{t('columnActions')}</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {sorted.map((file) => (
            <TableRow
              key={file.path + file.name}
              hover
              onContextMenu={(e) => onContextMenu(e, file)}
            >
              <TableCell padding="checkbox">
                {!file.isDirectory && (
                  <Checkbox
                    size="small"
                    checked={file.selected}
                    onChange={() => onSelect(file.name)}
                  />
                )}
              </TableCell>
              <TableCell sx={{ cursor: 'pointer', maxWidth: 520 }} onClick={() => file.isDirectory ? onFolder(file.path) : onOpen(file.path)}>
                <Stack direction="row" alignItems="center" spacing={1}>
                  <Box sx={{
                    width: 36,
                    height: 36,
                    flex: '0 0 auto',
                    borderRadius: 1,
                    overflow: 'hidden',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    bgcolor: 'action.hover',
                  }}>
                    {file.isVideo || file.isImage
                      ? <Preview file={file} thumbURL={thumbURL} />
                      : entryIcon(file)}
                  </Box>
                  <Typography variant="body2" noWrap title={file.name}>
                    {file.name}
                  </Typography>
                </Stack>
              </TableCell>
              <TableCell align="right" title={file.isDirectory ? '' : `${file.size} B`}>
                {file.isDirectory ? '—' : formatSize(file.size)}
              </TableCell>
              <TableCell>
                {file.name !== '..' && new Date(file.modTime).toLocaleString()}
              </TableCell>
              <TableCell>{typeLabel(file, t)}</TableCell>
              <TableCell align="right">
                <Tooltip title={t('download')}>
                  <IconButton size="small" onClick={() => onDownload(file.path)}>
                    <SaveAltIcon fontSize="small" />
                  </IconButton>
                </Tooltip>
                <Tooltip title={t('delete')}>
                  <IconButton size="small" onClick={() => onDelete(file)}>
                    <DeleteForeverIcon fontSize="small" />
                  </IconButton>
                </Tooltip>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  )
}

const FilesIcons: React.FC<ViewProps> = ({ files, t, thumbURL, onOpen, onFolder, onDownload, onDelete, onSelect, onContextMenu }) => (
  <Box
    sx={{
      display: 'grid',
      gap: 2,
      gridTemplateColumns: {
        xs: 'repeat(2, minmax(0, 1fr))',
        sm: 'repeat(3, minmax(0, 1fr))',
        md: 'repeat(4, minmax(0, 1fr))',
        lg: 'repeat(6, minmax(0, 1fr))',
      },
    }}
  >
    {files.map((file) => (
      <Card
        key={file.path + file.name}
        onContextMenu={(e) => onContextMenu(e, file)}
        sx={{ display: 'flex', flexDirection: 'column' }}
      >
        <CardActionArea onClick={() => file.isDirectory ? onFolder(file.path) : onOpen(file.path)}>
          <Box sx={{
            position: 'relative',
            width: '100%',
            pt: '56.25%',
            bgcolor: 'action.hover',
            overflow: 'hidden',
          }}>
            <Box sx={{ position: 'absolute', inset: 0 }}>
              <Preview file={file} thumbURL={thumbURL} />
            </Box>
          </Box>
          <CardContent sx={{ p: 1, '&:last-child': { pb: 1 } }}>
            <Typography variant="body2" noWrap title={file.name}>
              {file.name}
            </Typography>
            <Typography variant="caption" color="text.secondary" display="block" title={`${file.size} B`}>
              {file.isDirectory ? t('typeFolder') : formatSize(file.size)}
            </Typography>
            {file.name !== '..' && (
              <Typography variant="caption" color="text.secondary" display="block">
                {new Date(file.modTime).toLocaleString()}
              </Typography>
            )}
          </CardContent>
        </CardActionArea>
        <CardActions sx={{ p: 0.5, justifyContent: 'space-between', mt: 'auto' }}>
          {!file.isDirectory
            ? <Checkbox size="small" checked={file.selected} onChange={() => onSelect(file.name)} />
            : <Box />}
          <Box>
            <Tooltip title={t('download')}>
              <IconButton size="small" onClick={() => onDownload(file.path)}>
                <SaveAltIcon fontSize="small" />
              </IconButton>
            </Tooltip>
            <Tooltip title={t('delete')}>
              <IconButton size="small" onClick={() => onDelete(file)}>
                <DeleteForeverIcon fontSize="small" />
              </IconButton>
            </Tooltip>
          </Box>
        </CardActions>
      </Card>
    ))}
  </Box>
)

const IconMenu: React.FC<{
  posX: number
  posY: number
  hide: boolean
  onDownload: () => void
  onDelete: () => void
}> = ({ posX, posY, hide, onDelete, onDownload }) => {
  return (
    <Paper sx={{
      width: 320,
      maxWidth: '100%',
      position: 'absolute',
      top: posY,
      left: posX,
      display: hide ? 'none' : 'block',
      zIndex: (theme) => theme.zIndex.drawer + 1,
    }}>
      <MenuList>
        <MenuItem onClick={onDownload}>
          <ListItemIcon>
            <DownloadIcon fontSize="small" />
          </ListItemIcon>
          <ListItemText>
            Download
          </ListItemText>
        </MenuItem>
        <MenuItem onClick={onDelete}>
          <ListItemIcon>
            <DeleteForeverIcon fontSize="small" />
          </ListItemIcon>
          <ListItemText>
            Delete
          </ListItemText>
        </MenuItem>
      </MenuList>
    </Paper>
  )
}
