/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React from 'react';
import { Button, Typography } from '@douyinfe/semi-ui';
import { IconPlus, IconUpload } from '@douyinfe/semi-icons';
import CardPro from '../../common/ui/CardPro';
import { createCardProPagination } from '../../../helpers/utils';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
import { useChannelPreparationsData } from '../../../hooks/channels/useChannelPreparationsData';
import PreparationActions from './PreparationActions';
import PreparationFilters from './PreparationFilters';
import PreparationTable from './PreparationTable';
import EditPreparationModal from './modals/EditPreparationModal';
import ImportPreparationModal from './modals/ImportPreparationModal';
import ModelTestModal from '../channels/modals/ModelTestModal';
import AutoPromotionPanel from './AutoPromotionPanel';

const ChannelPreparationsPage = () => {
  const data = useChannelPreparationsData();
  const isMobile = useIsMobile();

  return (
    <>
      <EditPreparationModal
        visible={data.showEdit}
        preparation={data.editingPreparation}
        groupOptions={data.groupOptions}
        onCancel={data.closeEdit}
        onSubmit={data.savePreparation}
      />
      <ImportPreparationModal
        visible={data.showImport}
        groupOptions={data.groupOptions}
        onCancel={() => data.setShowImport(false)}
        onSubmit={data.importPreparations}
      />
      <ModelTestModal
        {...data}
        isMobile={isMobile}
        testChannel={data.testPreparation}
      />
      <AutoPromotionPanel t={data.t} refreshPreparations={data.refresh} />
      <CardPro
        type='type3'
        descriptionArea={
          <div className='flex flex-col md:flex-row md:items-center md:justify-between gap-3'>
            <div>
              <Typography.Title heading={5} style={{ margin: 0 }}>
                {data.t('渠道备货池')}
              </Typography.Title>
              <Typography.Text type='secondary'>
                {data.t(
                  '候选渠道只保存在备货池，不参与真实渠道调用，晋升后才会创建正式渠道。',
                )}
              </Typography.Text>
            </div>
            <div className='flex flex-col sm:flex-row gap-2 w-full md:w-auto'>
              <Button
                size='small'
                type='primary'
                icon={<IconPlus />}
                onClick={data.openCreate}
                className='w-full sm:w-auto'
              >
                {data.t('添加候选渠道')}
              </Button>
              <Button
                size='small'
                theme='outline'
                type='primary'
                icon={<IconUpload />}
                onClick={data.openImport}
                className='w-full sm:w-auto'
              >
                {data.t('导入候选渠道')}
              </Button>
            </div>
          </div>
        }
        actionsArea={<PreparationActions {...data} />}
        searchArea={<PreparationFilters {...data} />}
        paginationArea={createCardProPagination({
          currentPage: data.activePage,
          pageSize: data.pageSize,
          total: data.total,
          onPageChange: data.handlePageChange,
          onPageSizeChange: data.handlePageSizeChange,
          isMobile,
          t: data.t,
        })}
        t={data.t}
      >
        <PreparationTable {...data} />
      </CardPro>
    </>
  );
};

export default ChannelPreparationsPage;
